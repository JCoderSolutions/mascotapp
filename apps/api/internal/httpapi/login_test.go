package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gentleman/mascotapp/apps/api/internal/auth"
	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
	"github.com/gentleman/mascotapp/apps/api/internal/httpapi"
)

// Login is where three packages meet: password.go verifies the credential,
// token.go mints the access token, session.go starts the refresh family. Each is
// already tested on its own, so what these tests pin is the WIRING — and the
// wiring is where a login handler leaks.
//
// The rule this file was written under, learned from T-02-027's surviving
// mutant: assert the body of the REFUSAL, not only its status code. A handler
// that returns 401 with the reason attached passes every test that checks the
// number.

func loginHandler(t *testing.T, env *dbtest.Env) http.Handler {
	t.Helper()

	issuer, err := auth.NewTokenIssuer([]byte(testJWTSecret), testJWTIssuer, testJWTAudienc)
	if err != nil {
		t.Fatalf("building the token issuer: %v", err)
	}
	envelope, err := auth.NewEnvelope(testKEK)
	if err != nil {
		t.Fatalf("building the envelope: %v", err)
	}
	return httpapi.LoginHandler(env.AuthPool, issuer, time.Now, envelope)
}

// seedAccount creates a user who can complete login with a password alone.
//
// It deliberately does NOT register a shelter. Registration makes the caller an
// `owner`, and T-02-029 made a second factor mandatory for that role -- so a
// fixture built on registration would refuse every password-only login in this
// file, and the tests about cookies and claims would be asserting a 401.
//
// That is the requirement working, not a fixture problem: an adopter has no
// membership, browses the catalog, and applies. This is that shape.
// The privileged shape lives in login_totp_test.go, where it is the subject
// rather than the setup.
func seedAccount(t *testing.T, env *dbtest.Env, email, password string) {
	t.Helper()
	seedAdopter(t, env, email, password)
}

func postLogin(t *testing.T, handler http.Handler, email, password string) *httptest.ResponseRecorder {
	t.Helper()

	body := `{"email":"` + email + `","password":"` + password + `"}`
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body)))
	return rec
}

func TestLogin_IssuesATokenAndACookieOnTheCorrectPassword(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := loginHandler(t, env)

	const email, password = "good@refugio.test", "a-long-enough-password"
	seedAccount(t, env, email, password)

	rec := postLogin(t, handler, email, password)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding the response: %v", err)
	}
	if got.AccessToken == "" {
		t.Fatal("no access_token in the response")
	}

	// Verified through the issuer rather than by shape. A string with two dots
	// in it satisfies "looks like a JWT" and proves nothing about whether this
	// handler signed it with the key the middleware will check against.
	issuer, err := auth.NewTokenIssuer([]byte(testJWTSecret), testJWTIssuer, testJWTAudienc)
	if err != nil {
		t.Fatalf("building the verifier: %v", err)
	}
	claims, err := issuer.Verify(got.AccessToken, time.Now())
	if err != nil {
		t.Fatalf("the issued token does not verify: %v", err)
	}
	if claims.Subject.String() == "" {
		t.Error("the token carries no subject")
	}
	if want := int(auth.AccessTokenLifetime.Seconds()); got.ExpiresIn != want {
		t.Errorf("expires_in = %d, want %d", got.ExpiresIn, want)
	}
}

// TestLogin_SetsTheRefreshCookieWithEveryAttributeP2D9Requires spells the
// attributes out one at a time. A single assertion on the whole Set-Cookie
// string would pass or fail as a unit, and would not say WHICH attribute went
// missing — and each of these carries its own security property.
func TestLogin_SetsTheRefreshCookieWithEveryAttributeP2D9Requires(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := loginHandler(t, env)

	const email, password = "cookie@refugio.test", "a-long-enough-password"
	seedAccount(t, env, email, password)

	rec := postLogin(t, handler, email, password)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	raw := rec.Header().Get("Set-Cookie")
	if raw == "" {
		t.Fatal("no Set-Cookie header on a successful login")
	}

	for _, tt := range []struct {
		attribute string
		why       string
	}{
		{"__Secure-mascotapp_refresh=", "the __Secure- prefix means a non-HTTPS origin cannot set or overwrite this cookie"},
		{"HttpOnly", "without it, any XSS on the page reads the refresh token"},
		{"Secure", "required by the __Secure- prefix, and what keeps it off a cleartext hop"},
		{"SameSite=None", "the origins are separate by decision, so Strict or Lax would never send it (P2-D9)"},
		{"Path=/api/v1/auth", "Path=/ would attach the refresh token to every request, widening the blast radius for nothing"},
	} {
		if !strings.Contains(raw, tt.attribute) {
			t.Errorf("Set-Cookie is missing %q -- %s\n  got: %s", tt.attribute, tt.why, raw)
		}
	}
}

// TestLogin_RefusesTheWrongPasswordWithoutIssuingAnything is the pair to the
// test above: the refusal must not leave a session behind. Checking only the
// status code would pass for a handler that mints the cookie and then returns
// 401.
func TestLogin_RefusesTheWrongPasswordWithoutIssuingAnything(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := loginHandler(t, env)

	const email, password = "wrong@refugio.test", "a-long-enough-password"
	seedAccount(t, env, email, password)

	rec := postLogin(t, handler, email, "not-the-right-password")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	if cookie := rec.Header().Get("Set-Cookie"); cookie != "" {
		t.Errorf("a refused login set a cookie: %s", cookie)
	}
	if strings.Contains(rec.Body.String(), "access_token") {
		t.Errorf("a refused login returned a token: %s", rec.Body.String())
	}
}

// TestLogin_IsIndistinguishableBetweenAnUnknownEmailAndAWrongPassword is the
// enumeration property, and it is the reason this file exists in the shape it
// does. If the two refusals differ in status, body or headers, an
// unauthenticated caller can ask "is this address registered?" and get an
// answer.
//
// This is the same defect class as T-02-027's surviving mutant, one layer up:
// there the leak was the driver's error text, here it would be the handler's own
// helpfulness.
func TestLogin_IsIndistinguishableBetweenAnUnknownEmailAndAWrongPassword(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := loginHandler(t, env)

	const email, password = "known@refugio.test", "a-long-enough-password"
	seedAccount(t, env, email, password)

	wrongPassword := postLogin(t, handler, email, "not-the-right-password")
	unknownEmail := postLogin(t, handler, "nobody@refugio.test", password)

	if wrongPassword.Code != unknownEmail.Code {
		t.Errorf("status differs: wrong password %d, unknown email %d", wrongPassword.Code, unknownEmail.Code)
	}
	if wrongPassword.Body.String() != unknownEmail.Body.String() {
		t.Errorf("body differs:\n  wrong password: %s\n  unknown email:  %s",
			wrongPassword.Body.String(), unknownEmail.Body.String())
	}
	// Headers too. A `WWW-Authenticate` on one and not the other, or a
	// `Set-Cookie` clearing something, distinguishes the two just as loudly as
	// the body would.
	for name := range wrongPassword.Header() {
		if wrongPassword.Header().Get(name) != unknownEmail.Header().Get(name) {
			t.Errorf("header %s differs: %q vs %q",
				name, wrongPassword.Header().Get(name), unknownEmail.Header().Get(name))
		}
	}

	// Comparing the two refusals to EACH OTHER is only half the property, and
	// the mutation run proved it: both paths go through one function, so a
	// message changed to "no account with that email" changes both at once and
	// stays perfectly identical. The test passed while the handler told every
	// caller which check failed.
	//
	// So the content is asserted too. Each of these is a word that would answer
	// "is this address registered?" for a caller who only reads the body.
	body := strings.ToLower(wrongPassword.Body.String())
	for _, leak := range []string{"email", "account", "password", "user", "exist", "found", "unknown", "incorrect", "wrong"} {
		if strings.Contains(body, leak) {
			t.Errorf("the refusal body names %q, which says which check failed: %s", leak, wrongPassword.Body.String())
		}
	}
}

// TestLogin_SpendsTheSameWorkOnAnUnknownEmailAsOnAWrongPassword closes the
// channel the response body cannot show.
//
// Returning early when the address is not found makes an unknown email cost
// almost nothing while a known one costs a full Argon2id derivation — 64 MiB
// and three passes, about a tenth of a second. Both replies are byte-identical,
// and a caller with a stopwatch still reads the difference. Enumeration does not
// need a response body.
//
// The mutation run is why this exists: adding an early return SURVIVED every
// other test in this file, including the one that compares the two refusals.
//
// The assertion is a RATIO with wide tolerance, not a fixed budget. What it
// pins is the presence of the work, not how long the work takes — a floor in
// milliseconds would fail on a slower CI box and get "fixed" by loosening until
// it means nothing. The known-account path is the baseline, so both sides move
// together with the machine.
func TestLogin_SpendsTheSameWorkOnAnUnknownEmailAsOnAWrongPassword(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := loginHandler(t, env)

	const email, password = "timing@refugio.test", "a-long-enough-password"
	seedAccount(t, env, email, password)

	// Median of several attempts each. A single measurement on a shared CI
	// machine is a coin flip; the median discards the outlier that scheduling
	// produces without needing a statistics library.
	measure := func(email, password string) time.Duration {
		t.Helper()
		const attempts = 5
		samples := make([]time.Duration, 0, attempts)
		for range attempts {
			start := time.Now()
			rec := postLogin(t, handler, email, password)
			samples = append(samples, time.Since(start))
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected %d, got %d: %s", http.StatusUnauthorized, rec.Code, rec.Body.String())
			}
		}
		slices.Sort(samples)
		return samples[attempts/2]
	}

	knownAccount := measure(email, "not-the-right-password")
	unknownAccount := measure("nobody-here@refugio.test", password)

	// Ten percent of the known-account cost. An early return lands near zero
	// against a ~100ms Argon2id derivation, so this catches it with two orders
	// of magnitude to spare while tolerating any ordinary scheduling noise.
	floor := knownAccount / 10
	if unknownAccount < floor {
		t.Errorf("an unknown email took %v against %v for a known one (floor %v): the handler is returning before it hashes, which is a timing oracle over the user table",
			unknownAccount, knownAccount, floor)
	}
}

// TestLogin_NeverEchoesTheSubmittedPassword searches the whole response. Every
// other test here asserts what was issued; none would notice the credential
// coming back out in an error message.
func TestLogin_NeverEchoesTheSubmittedPassword(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := loginHandler(t, env)

	const email, password = "echo@refugio.test", "a-very-recognisable-password"
	seedAccount(t, env, email, password)

	for _, tt := range []struct {
		name     string
		email    string
		password string
	}{
		{"successful login", email, password},
		{"wrong password", email, "a-very-recognisable-wrong-one"},
		{"unknown email", "nobody-else@refugio.test", password},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := postLogin(t, handler, tt.email, tt.password)

			if strings.Contains(rec.Body.String(), tt.password) {
				t.Errorf("the response body contains the submitted password: %s", rec.Body.String())
			}
			for name, values := range rec.Header() {
				for _, v := range values {
					if strings.Contains(v, tt.password) {
						t.Errorf("header %s contains the submitted password", name)
					}
				}
			}
		})
	}
}

// TestLogin_RefusesMalformedInput keeps the handler from reaching Argon2id with
// nothing to verify. Hashing an empty password costs 64 MiB and three passes
// over it, on a 512 MiB instance — an unauthenticated caller should not be able
// to spend that by sending `{}`.
func TestLogin_RefusesMalformedInput(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := loginHandler(t, env)

	for _, tt := range []struct {
		name string
		body string
	}{
		{"not json", `not json`},
		{"no email", `{"password":"a-long-enough-password"}`},
		{"no password", `{"email":"someone@refugio.test"}`},
		{"both empty", `{"email":"","password":""}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(tt.body)))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
			if cookie := rec.Header().Get("Set-Cookie"); cookie != "" {
				t.Errorf("a rejected request set a cookie: %s", cookie)
			}
		})
	}
}

// TestLogin_DoesNotCarryATenantClaimBeforeAShelterIsChosen pins the tenant
// boundary at the point tokens are minted.
//
// A founder belongs to exactly one shelter, so the tempting shortcut is to put
// it straight into the token. That shortcut is what the shelter-exchange step
// exists to prevent: `ShelterID` is a POINTER precisely so "no shelter yet" is
// distinguishable from a shelter, and `uuid.Nil` would sail through the
// middleware's presence check and land in `WithTenant`.
//
// The login token asserts WHO you are. Which tenant you are acting as is a
// separate assertion, made after an explicit choice.
func TestLogin_DoesNotCarryATenantClaimBeforeAShelterIsChosen(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := loginHandler(t, env)

	const email, password = "tenantless@refugio.test", "a-long-enough-password"
	seedAccount(t, env, email, password)

	rec := postLogin(t, handler, email, password)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding the response: %v", err)
	}

	issuer, err := auth.NewTokenIssuer([]byte(testJWTSecret), testJWTIssuer, testJWTAudienc)
	if err != nil {
		t.Fatalf("building the verifier: %v", err)
	}
	claims, err := issuer.Verify(got.AccessToken, time.Now())
	if err != nil {
		t.Fatalf("the issued token does not verify: %v", err)
	}

	if claims.ShelterID != nil {
		t.Errorf("the login token carries shelter_id = %v, want none -- the tenant is chosen in a separate exchange, and this claim is the SOLE input to tenant resolution (ADR-0008)",
			*claims.ShelterID)
	}
}

// TestLogin_NamesTheUnscopedRoleRatherThanBorrowingATenantRole pins the way out
// of a real tension, so the next reader does not resolve it the other way.
//
// `AccessClaims.validate` refuses an empty role, correctly: RBAC reads that
// claim, and "" evaluates as "no permissions" in one reading and "unset, fall
// through" in another. But a role is a property of a MEMBERSHIP, and a
// membership belongs to one shelter — so a token with no `shelter_id` cannot
// honestly carry `owner` either.
//
// The tempting shortcut is to look up the founder's single membership and copy
// its role in. That produces a token asserting `owner` with no shelter named,
// which is exactly the shape the exchange step exists to prevent.
func TestLogin_NamesTheUnscopedRoleRatherThanBorrowingATenantRole(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := loginHandler(t, env)

	const email, password = "unscoped@refugio.test", "a-long-enough-password"
	seedAccount(t, env, email, password)

	rec := postLogin(t, handler, email, password)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding the response: %v", err)
	}

	issuer, err := auth.NewTokenIssuer([]byte(testJWTSecret), testJWTIssuer, testJWTAudienc)
	if err != nil {
		t.Fatalf("building the verifier: %v", err)
	}
	claims, err := issuer.Verify(got.AccessToken, time.Now())
	if err != nil {
		t.Fatalf("the issued token does not verify: %v", err)
	}

	if claims.Role != httpapi.RoleUnscoped {
		t.Errorf("role = %q, want %q -- the founder IS an owner, and this token must not say so before a shelter is chosen",
			claims.Role, httpapi.RoleUnscoped)
	}
	// Named explicitly: these are the values a shortcut would produce, and each
	// would sail through an RBAC check that only asks whether the role matches.
	for _, borrowed := range []string{"owner", "admin", "staff", "volunteer"} {
		if claims.Role == borrowed {
			t.Errorf("the login token borrowed the tenant role %q with no shelter_id to scope it", borrowed)
		}
	}
}

// TestLogin_RecordsThatOnlyAPasswordWasUsed pins `amr`. It is what lets a later
// route demand a second factor was actually EXERCISED rather than merely
// enrolled — the distinction T-02-029 turns into the phase's E2E criterion.
func TestLogin_RecordsThatOnlyAPasswordWasUsed(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := loginHandler(t, env)

	const email, password = "amr@refugio.test", "a-long-enough-password"
	seedAccount(t, env, email, password)

	rec := postLogin(t, handler, email, password)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding the response: %v", err)
	}

	issuer, err := auth.NewTokenIssuer([]byte(testJWTSecret), testJWTIssuer, testJWTAudienc)
	if err != nil {
		t.Fatalf("building the verifier: %v", err)
	}
	claims, err := issuer.Verify(got.AccessToken, time.Now())
	if err != nil {
		t.Fatalf("the issued token does not verify: %v", err)
	}

	if len(claims.AMR) != 1 || claims.AMR[0] != "pwd" {
		t.Errorf("amr = %v, want [pwd] -- a password-only login must not claim a factor it did not exercise", claims.AMR)
	}
}
