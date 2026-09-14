package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"

	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
	"github.com/gentleman/mascotapp/apps/api/internal/email"
	"github.com/gentleman/mascotapp/apps/api/internal/httpapi"
)

// The requirement here is an ABSENCE: a magic-link request must not reveal
// whether the address has an account. Absences are the shape this project has
// been bitten by most, so each channel is closed by its own test — status and
// body, timing, and the email itself.
//
// The email is the channel that has no HTTP equivalent. Two identical responses
// still leak if one of them sends mail and a person can observe their own inbox
// — which they can, because it is theirs.

// recordingSender is the Sender under test's watch. It records every send so
// the test can assert not only "one went out" but "exactly this address got one
// and no other did".
type recordingSender struct {
	mu   sync.Mutex
	sent []string
}

func (s *recordingSender) Send(ctx context.Context, to string, msg email.Message) error {
	// The port refuses a cancelled context (email.checkSend), so a double that
	// ignores it cannot see a handler that passes the request's own context to
	// a goroutine running after the reply. A test double that is more permissive
	// than the thing it stands in for makes a real bug untestable.
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if msg.Purpose != email.PurposeMagicLink {
		return nil
	}
	s.sent = append(s.sent, to)
	return nil
}

func (s *recordingSender) recipients() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.sent)
}

// testMagicLinkBase is where the mailed link points. The redemption endpoint it
// implies does not exist in this phase — see MagicLinkHandler's note — so this
// is the shape of a URL, not a route anybody serves yet.
const testMagicLinkBase = "https://mascotapp.pages.dev/auth/magic"

func magicLinkHandler(t *testing.T, env *dbtest.Env) (http.Handler, *recordingSender) {
	t.Helper()

	sender := &recordingSender{}
	return httpapi.MagicLinkHandler(env.AuthPool, sender, time.Now, testMagicLinkBase), sender
}

// waitForRecipients blocks until want emails have been recorded, or the deadline
// passes.
//
// The wait exists because the send is deliberately ASYNCHRONOUS: a synchronous
// one would make the existing-account path pay an SMTP round trip the other path
// does not, which is a timing oracle through two identical bodies. That design
// choice moves the cost into the test, and this is where it is paid.
//
// It returns whatever was recorded when it stopped waiting, so a caller can
// assert "exactly one" and also catch "two".
func (s *recordingSender) waitForRecipients(t *testing.T, want int) []string {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for {
		got := s.recipients()
		if len(got) >= want || time.Now().After(deadline) {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func postMagicLink(t *testing.T, handler http.Handler, address string) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec,
		httptest.NewRequest(http.MethodPost, "/api/v1/auth/magic-link", strings.NewReader(`{"email":"`+address+`"}`)))
	return rec
}

// TestMagicLink_TheResponseIsIdenticalWhetherOrNotTheAccountExists is the
// spec's own scenario. It asserts equality between the two responses AND the
// content of one, because the two go through the same code path: a body changed
// to say "if that address exists, we sent a link" would change both at once and
// stay perfectly identical.
func TestMagicLink_TheResponseIsIdenticalWhetherOrNotTheAccountExists(t *testing.T) {
	env := dbtest.Postgres(t)
	handler, _ := magicLinkHandler(t, env)

	const known = "magic-known@refugio.test"
	seedAdopter(t, env, known, "a-long-enough-password")

	existing := postMagicLink(t, handler, known)
	missing := postMagicLink(t, handler, "magic-nobody@refugio.test")

	if existing.Code != missing.Code {
		t.Errorf("status differs: existing %d, missing %d", existing.Code, missing.Code)
	}
	if existing.Body.String() != missing.Body.String() {
		t.Errorf("body differs:\n  existing: %s\n  missing:  %s", existing.Body.String(), missing.Body.String())
	}
	for name := range existing.Header() {
		if existing.Header().Get(name) != missing.Header().Get(name) {
			t.Errorf("header %s differs: %q vs %q",
				name, existing.Header().Get(name), missing.Header().Get(name))
		}
	}

	// And the content. Each of these is a word that answers "does this address
	// have an account?" for a caller who reads only the body.
	body := strings.ToLower(existing.Body.String())
	for _, leak := range []string{"exist", "found", "unknown", "account", "registered", "no such", "sent you"} {
		if strings.Contains(body, leak) {
			t.Errorf("the response body says %q, which answers whether the account exists: %s",
				leak, existing.Body.String())
		}
	}
}

// TestMagicLink_OnlyTheExistingAddressReceivesAnEmail closes the channel HTTP
// cannot show. Two identical responses still leak when one of them sends mail:
// the person owns the inbox, so they observe the difference directly.
func TestMagicLink_OnlyTheExistingAddressReceivesAnEmail(t *testing.T) {
	env := dbtest.Postgres(t)
	handler, sender := magicLinkHandler(t, env)

	const known = "magic-onlyone@refugio.test"
	const unknown = "magic-noone@refugio.test"
	seedAdopter(t, env, known, "a-long-enough-password")

	postMagicLink(t, handler, known)
	postMagicLink(t, handler, unknown)

	// The WAIT is what this call is for; its value is deliberately discarded.
	// Reading the recipients here would assert "at least one had not gone out
	// yet", which a handler mailing BOTH addresses satisfies on its way past.
	_ = sender.waitForRecipients(t, 1)
	time.Sleep(250 * time.Millisecond)
	got := sender.recipients()

	if len(got) != 1 {
		t.Fatalf("emails sent = %d (%v), want exactly 1", len(got), got)
	}
	if got[0] != known {
		t.Errorf("the email went to %q, want %q", got[0], known)
	}
	if slices.Contains(got, unknown) {
		t.Error("an address with no account received a magic link")
	}
}

// TestMagicLink_SpendsComparableWorkOnBothPaths closes the timing channel, and
// it is the one the response body cannot show.
//
// The trap is the reverse of login's. There, the work was real (Argon2id) and
// the risk was skipping it; here the expensive step is SENDING MAIL, which must
// happen for one address and not the other. So the paths cannot be made
// identical by doing the same work — they are made comparable by doing the
// lookup and the reply the same way, and by the send not blocking the response.
//
// The assertion is a ratio with wide tolerance in BOTH directions: the unknown
// path must not be much faster (it skipped a lookup) and the known path must
// not be much slower (it waited for the mail).
func TestMagicLink_SpendsComparableWorkOnBothPaths(t *testing.T) {
	env := dbtest.Postgres(t)
	handler, _ := magicLinkHandler(t, env)

	const known = "magic-timing@refugio.test"
	seedAdopter(t, env, known, "a-long-enough-password")

	measure := func(address string) time.Duration {
		t.Helper()
		const attempts = 5
		samples := make([]time.Duration, 0, attempts)
		for range attempts {
			start := time.Now()
			postMagicLink(t, handler, address)
			samples = append(samples, time.Since(start))
		}
		slices.Sort(samples)
		return samples[attempts/2]
	}

	existing := measure(known)
	missing := measure("magic-notiming@refugio.test")

	// Four times either way. Generous on purpose: what this catches is an order
	// of magnitude — a path that skipped the database entirely, or one that
	// blocked on an SMTP round trip. A tight bound here would fail on a busy
	// machine and get loosened until it meant nothing.
	if existing > missing*4 {
		t.Errorf("the existing-account path took %v against %v for a missing one: the difference is observable",
			existing, missing)
	}
	if missing > existing*4 {
		t.Errorf("the missing-account path took %v against %v for an existing one: the difference is observable",
			missing, existing)
	}
}

// slowSender models the only sender that matters here: a real one. Delivery is
// an HTTP call to a mail provider, and the in-memory doubles above return in
// microseconds — which is exactly what hid this property until a mutant went
// looking for it.
type slowSender struct {
	delay time.Duration
	mu    sync.Mutex
	sent  []string
}

func (s *slowSender) Send(ctx context.Context, to string, _ email.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	time.Sleep(s.delay)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, to)
	return nil
}

// TestMagicLink_TheReplyDoesNotWaitForTheSend is the test the fast doubles could
// not be: with an in-memory sender, a SYNCHRONOUS send costs nothing and the
// timing comparison above sees no difference at all. The mutation run found
// exactly that gap — removing the goroutine survived every other test in this
// file.
//
// In production the send is an HTTP call to a mail provider. If the reply waits
// for it, the existing-account path costs a round trip the missing-account path
// never pays, and that difference is readable through two identical bodies by
// anyone holding a stopwatch. The property is not "the send is fast"; it is
// "the reply does not depend on the send", and only a SLOW sender can tell them
// apart.
//
// The sender delays rather than blocking on a channel on purpose: a channel a
// synchronous handler never reaches would hang this test instead of failing it,
// and a hang reports nothing.
func TestMagicLink_TheReplyDoesNotWaitForTheSend(t *testing.T) {
	env := dbtest.Postgres(t)

	const delay = 500 * time.Millisecond
	sender := &slowSender{delay: delay}
	handler := httpapi.MagicLinkHandler(env.AuthPool, sender, time.Now, testMagicLinkBase)

	const known = "magic-nowait@refugio.test"
	seedAdopter(t, env, known, "a-long-enough-password")

	start := time.Now()
	rec := postMagicLink(t, handler, known)
	elapsed := time.Since(start)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if elapsed >= delay {
		t.Errorf("the reply took %v against a %v send: it waited for the mail, so the existing-account path is observably slower than the missing-account one",
			elapsed, delay)
	}
}

// TestMagicLink_TheSendSurvivesTheHandlerReturning runs through a REAL server,
// and that is the entire point of it.
//
// net/http cancels a request's context when ServeHTTP returns.
// httptest.NewRecorder does not — it never had a server to do it — so a handler
// that hands the request's own context to a goroutine outliving the reply looks
// perfectly healthy under every other test in this file and mails nobody in
// production. The mutation run found exactly that: swapping
// context.WithoutCancel for the request context survived all of them.
//
// The failure it guards against is the worst shape available: no error, no log,
// a 202 to the caller, and no email ever.
func TestMagicLink_TheSendSurvivesTheHandlerReturning(t *testing.T) {
	env := dbtest.Postgres(t)

	sender := &recordingSender{}
	server := httptest.NewServer(httpapi.MagicLinkHandler(env.AuthPool, sender, time.Now, testMagicLinkBase))
	defer server.Close()

	const known = "magic-realserver@refugio.test"
	seedAdopter(t, env, known, "a-long-enough-password")

	resp, err := http.Post(server.URL, "application/json", strings.NewReader(`{"email":"`+known+`"}`))
	if err != nil {
		t.Fatalf("posting to the test server: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusAccepted)
	}

	got := sender.waitForRecipients(t, 1)
	if len(got) != 1 || got[0] != known {
		t.Errorf("recipients = %v, want exactly [%s]: the send did not survive the handler returning",
			got, known)
	}
}

// TestMagicLink_ASuspendedAccountGetsNoLink pins the half of the condition the
// mutation run found unasserted. Every account in this file is active, so
// loosening the check to "the row exists" changed nothing.
//
// A suspended account is suspended for a reason — and a magic link is a login.
// Mailing one hands back the access the suspension removed, without a password
// and without a second factor.
//
// The reply is still the ordinary one: a caller who learns their suspension from
// a different response learns it from an endpoint that answers to anybody.
func TestMagicLink_ASuspendedAccountGetsNoLink(t *testing.T) {
	env := dbtest.Postgres(t)
	handler, sender := magicLinkHandler(t, env)

	const suspended = "magic-suspended@refugio.test"
	userID := seedAdopter(t, env, suspended, "a-long-enough-password")
	// Through the OWNER pool: app_auth holds UPDATE on the credential columns
	// of `users` and deliberately not on `status` (00013).
	dbtest.MustExec(t, env.OwnerPool, `UPDATE users SET status = 'suspended' WHERE id = $1`, userID)

	rec := postMagicLink(t, handler, suspended)

	if rec.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d: a suspended account got a different reply, which tells the caller about the suspension",
			rec.Code, http.StatusAccepted)
	}

	time.Sleep(250 * time.Millisecond)
	if got := sender.recipients(); len(got) != 0 {
		t.Errorf("a suspended account received a magic link: %v", got)
	}
}

// TestMagicLink_RefusesMalformedInputWithoutRevealingAnything keeps the
// validation path from becoming its own oracle. A 400 for a malformed address
// is fine — that is a property of the INPUT, which the caller already knows. A
// 400 that only happens for unregistered addresses would not be.
func TestMagicLink_RefusesMalformedInputWithoutRevealingAnything(t *testing.T) {
	env := dbtest.Postgres(t)
	handler, sender := magicLinkHandler(t, env)

	for _, tt := range []struct {
		name string
		body string
	}{
		{"not json", `not json`},
		{"no email field", `{}`},
		{"empty email", `{"email":""}`},
		{"whitespace only", `{"email":"   "}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec,
				httptest.NewRequest(http.MethodPost, "/api/v1/auth/magic-link", strings.NewReader(tt.body)))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}

	// A pause before asserting the absence. The send is asynchronous, so
	// checking immediately would pass for a handler that mails malformed
	// addresses slowly.
	time.Sleep(250 * time.Millisecond)
	if got := sender.recipients(); len(got) != 0 {
		t.Errorf("a malformed request sent mail to %v", got)
	}
}

// TestMagicLink_TheEmailCarriesASingleUseCredentialAndTheResponseDoesNot is the
// anti-vacuity case for the whole file.
//
// Every test above is satisfied by a handler that sends an email with an empty
// body, and one of them is satisfied by a handler that sends nothing at all to
// anybody. This asserts the link exists, and that it exists ONLY in the mail:
// returning it in the response would hand the credential to whoever made the
// request rather than to whoever owns the address, which is the entire point of
// mailing it.
func TestMagicLink_TheEmailCarriesASingleUseCredentialAndTheResponseDoesNot(t *testing.T) {
	env := dbtest.Postgres(t)

	captured := &capturingSender{}
	handler := httpapi.MagicLinkHandler(env.AuthPool, captured, time.Now, testMagicLinkBase)

	const known = "magic-body@refugio.test"
	seedAdopter(t, env, known, "a-long-enough-password")

	rec := postMagicLink(t, handler, known)

	body := captured.waitForBody(t)
	if body == "" {
		t.Fatal("the magic-link email has an empty body")
	}
	token := captured.token(t)
	if len(token) < 32 {
		t.Errorf("the link's token is %d characters, which is too short to be unguessable: %q",
			len(token), token)
	}
	if strings.Contains(rec.Body.String(), token) {
		t.Errorf("the HTTP response contains the token, which hands the credential to the requester rather than to the address owner: %s",
			rec.Body.String())
	}
}

// capturingSender keeps the last message so the body can be inspected. It is
// separate from recordingSender on purpose: the tests about indistinguishability
// must not have the body in reach, or one of them could accidentally assert
// against it and stop being about the response.
type capturingSender struct {
	mu   sync.Mutex
	last email.Message
}

func (s *capturingSender) Send(ctx context.Context, _ string, msg email.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last = msg
	return nil
}

func (s *capturingSender) lastBody() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last.Body
}

// waitForBody blocks until a message has been captured, for the same reason
// waitForRecipients does.
func (s *capturingSender) waitForBody(t *testing.T) string {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for {
		if body := s.lastBody(); body != "" || time.Now().After(deadline) {
			return body
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// token pulls the opaque part out of the link in the body.
//
// It anchors on the known base and stops at the first whitespace. The obvious
// version — everything after the last "/" — swallows the prose that follows the
// link, so ANY token measured long enough and a short one was unassertable. The
// mutation run is what found that: an eight-character token survived.
func (s *capturingSender) token(t *testing.T) string {
	t.Helper()

	body := s.lastBody()
	prefix := testMagicLinkBase + "/"
	index := strings.Index(body, prefix)
	if index < 0 {
		t.Fatalf("no link starting with %q in the magic-link body: %q", prefix, body)
	}

	rest := body[index+len(prefix):]
	if cut := strings.IndexFunc(rest, unicode.IsSpace); cut >= 0 {
		rest = rest[:cut]
	}
	return rest
}
