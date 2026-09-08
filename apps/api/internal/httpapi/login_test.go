package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	return httpapi.LoginHandler(env.AuthPool, issuer, time.Now)
}

// seedAccount registers one so login has something to find. Registration is
// already tested; using it here rather than inserting rows by hand means these
// tests break if the two handlers ever disagree about how a credential is
// stored, which is a disagreement no unit test on either side would catch.
func seedAccount(t *testing.T, env *dbtest.Env, email, password string) {
	t.Helper()

	body := `{
		"email": "` + email + `",
		"password": "` + password + `",
		"full_name": "Login Fixture",
		"shelter_name": "Refugio Login ` + email + `",
		"shelter_slug": "refugio-login-` + strings.Split(email, "@")[0] + `"
	}`
	rec := httptest.NewRecorder()
	httpapi.RegisterHandler(env.AuthPool, env.TenantPool).
		ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body)))

	if rec.Code != http.StatusCreated {
		t.Fatalf("seeding %s: status = %d, want %d: %s", email, rec.Code, http.StatusCreated, rec.Body.String())
	}
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
