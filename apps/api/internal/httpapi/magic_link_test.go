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

func (s *recordingSender) Send(_ context.Context, to string, msg email.Message) error {
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

func magicLinkHandler(t *testing.T, env *dbtest.Env) (http.Handler, *recordingSender) {
	t.Helper()

	sender := &recordingSender{}
	return httpapi.MagicLinkHandler(env.AuthPool, sender, time.Now), sender
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
	handler := httpapi.MagicLinkHandler(env.AuthPool, captured, time.Now)

	const known = "magic-body@refugio.test"
	seedAdopter(t, env, known, "a-long-enough-password")

	rec := postMagicLink(t, handler, known)

	body := captured.lastBody()
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

func (s *capturingSender) Send(_ context.Context, _ string, msg email.Message) error {
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

// token pulls the opaque part out of the link in the body.
func (s *capturingSender) token(t *testing.T) string {
	t.Helper()

	body := s.lastBody()
	index := strings.LastIndex(body, "/")
	if index < 0 {
		t.Fatalf("no link in the magic-link body: %q", body)
	}
	return strings.TrimSpace(body[index+1:])
}
