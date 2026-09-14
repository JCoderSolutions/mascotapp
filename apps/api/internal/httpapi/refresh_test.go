package httpapi_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gentleman/mascotapp/apps/api/internal/auth"
	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
	"github.com/gentleman/mascotapp/apps/api/internal/httpapi"
)

// Refresh and logout are the two routes the refresh cookie is sent to, and the
// cookie is sent with SameSite=None. That makes them the two routes a
// cross-site page can trigger with the user's session attached, which is why
// the origin check is not one check among several: it is the first thing that
// runs, and the tests below assert the ORDER, not only the outcome.
//
// Order is observable here without a spy. A request that fails the origin
// check AND carries a malformed cookie answers 403 if the origin ran first and
// 401 if the cookie did. And a request that fails the origin check with a VALID
// cookie leaves that cookie spendable afterwards — which it would not be if the
// rotation had run before the refusal.

func sessionHandlers(t *testing.T, env *dbtest.Env) (refresh, logout http.Handler) {
	t.Helper()

	issuer, err := auth.NewTokenIssuer([]byte(testJWTSecret), testJWTIssuer, testJWTAudienc)
	if err != nil {
		t.Fatalf("building the token issuer: %v", err)
	}
	allowlist, err := httpapi.NewOriginAllowlist([]string{allowedWebOrigin})
	if err != nil {
		t.Fatalf("building the origin allowlist: %v", err)
	}

	return httpapi.RefreshHandler(env.AuthPool, issuer, allowlist, time.Now),
		httpapi.LogoutHandler(env.AuthPool, allowlist)
}

// loginForCookie logs a fresh adopter in and returns the refresh cookie value
// the server set. Going through the real login handler rather than inserting a
// row means these tests break if login and refresh ever disagree about the
// cookie's format — which no unit test on either side would notice.
func loginForCookie(t *testing.T, env *dbtest.Env, email string) string {
	t.Helper()

	const password = "a-long-enough-password"
	seedAdopter(t, env, email, password)

	rec := postLogin(t, loginHandler(t, env), email, password)
	if rec.Code != http.StatusOK {
		t.Fatalf("logging %s in: status = %d: %s", email, rec.Code, rec.Body.String())
	}
	return cookieValue(t, rec)
}

func cookieValue(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()

	for _, c := range rec.Result().Cookies() {
		if c.Name == refreshCookieNameForTest {
			return c.Value
		}
	}
	t.Fatalf("no %s cookie in the response; Set-Cookie was %q", refreshCookieNameForTest, rec.Header().Get("Set-Cookie"))
	return ""
}

// postWithCookie sends a POST from origin carrying cookie. An empty origin
// omits the header entirely, and an empty cookie omits the cookie.
func postWithCookie(t *testing.T, handler http.Handler, path, origin, cookie string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, path, nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: refreshCookieNameForTest, Value: cookie})
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// --- the origin check runs first --------------------------------------------

func TestRefresh_ChecksTheOriginBeforeTheCookie(t *testing.T) {
	env := dbtest.Postgres(t)
	refresh, _ := sessionHandlers(t, env)

	rec := postWithCookie(t, refresh, "/api/v1/auth/refresh", evilOrigin, "not-a-cookie-this-server-issued")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d -- a %d here means the malformed cookie was parsed before the origin was checked",
			rec.Code, http.StatusForbidden, rec.Code)
	}
}

// TestRefresh_ARefusedOriginDoesNotSpendTheToken is the stronger form of the
// test above. A status code shows which check answered; this shows the
// rotation never ran, because the cookie still works afterwards.
func TestRefresh_ARefusedOriginDoesNotSpendTheToken(t *testing.T) {
	env := dbtest.Postgres(t)
	refresh, _ := sessionHandlers(t, env)

	cookie := loginForCookie(t, env, "refresh-unspent@refugio.test")

	if rec := postWithCookie(t, refresh, "/api/v1/auth/refresh", evilOrigin, cookie); rec.Code != http.StatusForbidden {
		t.Fatalf("a cross-site refresh: status = %d, want %d", rec.Code, http.StatusForbidden)
	}

	rec := postWithCookie(t, refresh, "/api/v1/auth/refresh", allowedWebOrigin, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("the same cookie from the allowed origin: status = %d, want %d -- the refused request spent it: %s",
			rec.Code, http.StatusOK, rec.Body.String())
	}
}

// --- rotation, end to end ----------------------------------------------------

func TestRefresh_RotatesTheCookieAndIssuesAnAccessToken(t *testing.T) {
	env := dbtest.Postgres(t)
	refresh, _ := sessionHandlers(t, env)

	cookie := loginForCookie(t, env, "refresh-rotates@refugio.test")

	rec := postWithCookie(t, refresh, "/api/v1/auth/refresh", allowedWebOrigin, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	rotated := cookieValue(t, rec)
	if rotated == cookie {
		t.Error("the refresh returned the same cookie value it was given: nothing rotated")
	}

	raw := rec.Header().Get("Set-Cookie")
	for _, attribute := range []string{"HttpOnly", "Secure", "SameSite=None", "Path=/api/v1/auth"} {
		if !strings.Contains(raw, attribute) {
			t.Errorf("the rotated cookie is missing %q -- refresh must set it exactly as login does: %s", attribute, raw)
		}
	}

	claims := accessTokenFrom(t, rec)
	if claims.ShelterID != nil {
		t.Errorf("the refreshed token carries shelter_id = %v, want none: refresh re-issues identity, it does not choose a tenant",
			*claims.ShelterID)
	}
	if claims.Role != httpapi.RoleUnscoped {
		t.Errorf("role = %q, want %q", claims.Role, httpapi.RoleUnscoped)
	}
}

// TestRefresh_ReusingASpentCookieRevokesTheWholeFamily is the spec's scenario
// end to end. Rotating once leaves the old cookie spent; presenting it again is
// either the user replaying a stale value or a thief with a copy, and the server
// cannot tell which — so the whole family dies, including the cookie the
// legitimate holder is using right now.
//
// The last assertion is the one that proves containment rather than refusal.
// Refusing the stale cookie is easy; the new cookie dying too is the property.
func TestRefresh_ReusingASpentCookieRevokesTheWholeFamily(t *testing.T) {
	env := dbtest.Postgres(t)
	refresh, _ := sessionHandlers(t, env)

	original := loginForCookie(t, env, "refresh-reuse@refugio.test")

	first := postWithCookie(t, refresh, "/api/v1/auth/refresh", allowedWebOrigin, original)
	if first.Code != http.StatusOK {
		t.Fatalf("the first rotation: status = %d: %s", first.Code, first.Body.String())
	}
	current := cookieValue(t, first)

	if rec := postWithCookie(t, refresh, "/api/v1/auth/refresh", allowedWebOrigin, original); rec.Code != http.StatusUnauthorized {
		t.Fatalf("replaying the spent cookie: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	rec := postWithCookie(t, refresh, "/api/v1/auth/refresh", allowedWebOrigin, current)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("the family's CURRENT cookie still works after reuse was detected: status = %d, want %d -- the thief's copy was refused and the session it came from was not contained",
			rec.Code, http.StatusUnauthorized)
	}
}

// TestRefresh_NeverClaimsASecondFactor pins the user's decision of 2026-09-14.
//
// `refresh_tokens` does not record how a session was authenticated, so a
// refresh cannot know whether TOTP was used at login. Preserving `otp` would
// need that stored, and it would make a stolen refresh cookie carry the second
// factor for the life of its family. Emitting `[pwd]` means a refresh never
// asserts a factor it did not itself verify: a thief holding the cookie gets a
// token that every route demanding `otp` refuses.
//
// The account here is an OWNER who logged in WITH a valid TOTP code, so the
// login token genuinely carried `otp`. That is the case where preserving it
// would be tempting, and the case this test exists for.
func TestRefresh_NeverClaimsASecondFactor(t *testing.T) {
	env := dbtest.Postgres(t)
	refresh, _ := sessionHandlers(t, env)

	const email, password = "refresh-owner@refugio.test", "a-long-enough-password"
	userID := registerOwner(t, env, email, password, "refresh-owner")
	secret := enrolTOTP(t, env, userID)

	login := postLoginWith(t, totpLoginHandler(t, env),
		`{"email":"`+email+`","password":"`+password+`","totp_code":"`+currentTOTP(t, secret)+`"}`)
	if login.Code != http.StatusOK {
		t.Fatalf("the two-factor login: status = %d: %s", login.Code, login.Body.String())
	}
	if loginClaims := accessTokenFrom(t, login); len(loginClaims.AMR) != 2 {
		t.Fatalf("precondition: the login token should carry [pwd otp], got %v", loginClaims.AMR)
	}

	rec := postWithCookie(t, refresh, "/api/v1/auth/refresh", allowedWebOrigin, cookieValue(t, login))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	claims := accessTokenFrom(t, rec)
	if len(claims.AMR) != 1 || claims.AMR[0] != "pwd" {
		t.Errorf("amr = %v after refresh, want [pwd] -- a refresh verified no second factor and must not claim one", claims.AMR)
	}
}

// TestRefresh_EveryRefusalLooksTheSame compares the refusals to each other AND
// asserts the content of one. The comparison alone cannot see a message that
// names the failing reason on every path, because they share one helper.
//
// Reuse is in the set on purpose. It is the refusal the SERVER must be able to
// distinguish — it raises an alarm — and the one the CALLER must not: a thief
// told "this token was already used" learns the legitimate user is active.
func TestRefresh_EveryRefusalLooksTheSame(t *testing.T) {
	env := dbtest.Postgres(t)
	refresh, _ := sessionHandlers(t, env)

	spent := loginForCookie(t, env, "refresh-refusals@refugio.test")
	if rec := postWithCookie(t, refresh, "/api/v1/auth/refresh", allowedWebOrigin, spent); rec.Code != http.StatusOK {
		t.Fatalf("spending the cookie: status = %d: %s", rec.Code, rec.Body.String())
	}

	refusals := map[string]*httptest.ResponseRecorder{
		"no cookie":        postWithCookie(t, refresh, "/api/v1/auth/refresh", allowedWebOrigin, ""),
		"malformed cookie": postWithCookie(t, refresh, "/api/v1/auth/refresh", allowedWebOrigin, "garbage"),
		"reused cookie":    postWithCookie(t, refresh, "/api/v1/auth/refresh", allowedWebOrigin, spent),
	}

	reference := refusals["no cookie"]
	for name, rec := range refusals {
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want %d", name, rec.Code, http.StatusUnauthorized)
		}
		if rec.Body.String() != reference.Body.String() {
			t.Errorf("%s: body %q differs from %q", name, rec.Body.String(), reference.Body.String())
		}
		if cookie := rec.Header().Get("Set-Cookie"); strings.Contains(cookie, refreshCookieNameForTest+"=") &&
			!strings.Contains(cookie, "Max-Age=0") {
			t.Errorf("%s: a refused refresh set a live cookie: %s", name, cookie)
		}
	}

	body := strings.ToLower(reference.Body.String())
	for _, leak := range []string{"reuse", "revoked", "expired", "already", "family", "malformed", "missing"} {
		if strings.Contains(body, leak) {
			t.Errorf("the refusal names %q, which tells the caller why: %s", leak, reference.Body.String())
		}
	}
}

// TestRefresh_AZeroUserPrefixIsRefusedLikeAnyOtherBadCookie covers a cookie
// that PARSES. Sixteen zero bytes are a valid uuid encoding, so
// ParseRefreshCookie hands back uuid.Nil without complaint, and WithAuthUser is
// what refuses it -- with ErrNoAuthUser, an error rather than a refusal.
//
// Answered as a 500, that is a reply no other bad cookie produces, from a value
// anyone can construct. The mutation run found the branch that prevents it
// unasserted. Logout has the same branch and gets the same cookie.
func TestRefresh_AZeroUserPrefixIsRefusedLikeAnyOtherBadCookie(t *testing.T) {
	env := dbtest.Postgres(t)
	refresh, logout := sessionHandlers(t, env)

	zero := auth.FormatRefreshCookie(uuid.Nil, bytes.Repeat([]byte{0x01}, auth.RefreshSecretLength))

	rec := postWithCookie(t, refresh, "/api/v1/auth/refresh", allowedWebOrigin, zero)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("refresh with a zero-uuid prefix: status = %d, want %d: %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	reference := postWithCookie(t, refresh, "/api/v1/auth/refresh", allowedWebOrigin, "garbage")
	if rec.Body.String() != reference.Body.String() {
		t.Errorf("the zero-uuid refusal %q differs from an ordinary bad cookie's %q", rec.Body.String(), reference.Body.String())
	}

	if out := postWithCookie(t, logout, "/api/v1/auth/logout", allowedWebOrigin, zero); out.Code != http.StatusNoContent {
		t.Errorf("logout with a zero-uuid prefix: status = %d, want %d: %s", out.Code, http.StatusNoContent, out.Body.String())
	}
}

// TestRefresh_ConcurrentUsesOfOneCookieNeverAnswer500 covers the other half of
// the branch the zero-uuid test covers, and the half no single request reaches.
//
// Several requests presenting one cookie at once read it live before any of
// them holds the family lock. One wins. A loser whose read was taken before the
// winner committed finds the row already revoked by the time it tries to revoke
// it, and RotateRefreshToken reports that as an ERROR wrapping
// ErrRefreshTokenInvalid -- not as a refusal -- because nothing was written and
// the caller's rollback has nothing to undo. Answered as a 500, a double-clicked
// refresh would surface a server error to a user who did nothing wrong.
//
// The assertion is not which request wins. It is that every response is a 200
// or a 401, and that exactly one is a 200. Trials repeat for the same reason the
// auth package's concurrency test repeats: a single attempt at a timing-dependent
// path proves nothing.
func TestRefresh_ConcurrentUsesOfOneCookieNeverAnswer500(t *testing.T) {
	env := dbtest.Postgres(t)
	refresh, _ := sessionHandlers(t, env)

	const trials = 10
	const racers = 6

	for trial := range trials {
		cookie := loginForCookie(t, env, fmt.Sprintf("refresh-race-%d@refugio.test", trial))

		codes := make([]int, racers)
		var ready, done sync.WaitGroup
		ready.Add(racers)
		done.Add(racers)
		start := make(chan struct{})

		for i := range racers {
			go func() {
				defer done.Done()
				ready.Done()
				<-start
				codes[i] = postWithCookie(t, refresh, "/api/v1/auth/refresh", allowedWebOrigin, cookie).Code
			}()
		}

		// Every racer blocks on `start` until all of them have reached it, so
		// closing the channel releases them as close to simultaneously as this
		// process can arrange.
		ready.Wait()
		close(start)
		done.Wait()

		wins := 0
		for racer, code := range codes {
			switch code {
			case http.StatusOK:
				wins++
			case http.StatusUnauthorized:
			default:
				t.Fatalf("trial %d, racer %d: status = %d, want %d or %d -- a lost concurrent rotation surfaced as a server error",
					trial, racer, code, http.StatusOK, http.StatusUnauthorized)
			}
		}
		if wins != 1 {
			t.Fatalf("trial %d: %d of %d concurrent refreshes of one cookie succeeded, want exactly 1 (codes %v)",
				trial, wins, racers, codes)
		}
	}
}

// --- logout ------------------------------------------------------------------

func TestLogout_RevokesThePresentedToken(t *testing.T) {
	env := dbtest.Postgres(t)
	refresh, logout := sessionHandlers(t, env)

	cookie := loginForCookie(t, env, "logout-revokes@refugio.test")

	rec := postWithCookie(t, logout, "/api/v1/auth/logout", allowedWebOrigin, cookie)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	// The browser is told to drop the cookie. Revoking server-side is the
	// control; clearing client-side is so the next page load does not present a
	// dead credential and read the 401 as a bug.
	cleared := rec.Header().Get("Set-Cookie")
	if !strings.Contains(cleared, refreshCookieNameForTest+"=") || !strings.Contains(cleared, "Max-Age=0") {
		t.Errorf("logout did not clear the refresh cookie: Set-Cookie = %q", cleared)
	}

	if after := postWithCookie(t, refresh, "/api/v1/auth/refresh", allowedWebOrigin, cookie); after.Code != http.StatusUnauthorized {
		t.Errorf("the logged-out cookie still refreshes: status = %d, want %d", after.Code, http.StatusUnauthorized)
	}
}

// TestLogout_EndsTheWholeSessionEvenFromAStaleCookie pins why logout revokes
// the FAMILY rather than only the row it was handed.
//
// Rotation leaves exactly one live row per family, and it is not the one an
// older cookie points at. Revoking only the presented row therefore does
// nothing when the cookie is stale — and the successor, the credential that
// actually opens the session, survives the logout.
//
// The same shape arrives without anyone holding an old cookie: a refresh in
// flight while the user logs out inserts a successor that a single-row revoke
// never sees. Revoking the family under the family lock is what closes both.
func TestLogout_EndsTheWholeSessionEvenFromAStaleCookie(t *testing.T) {
	env := dbtest.Postgres(t)
	refresh, logout := sessionHandlers(t, env)

	stale := loginForCookie(t, env, "logout-stale@refugio.test")

	rotated := postWithCookie(t, refresh, "/api/v1/auth/refresh", allowedWebOrigin, stale)
	if rotated.Code != http.StatusOK {
		t.Fatalf("rotating: status = %d: %s", rotated.Code, rotated.Body.String())
	}
	current := cookieValue(t, rotated)

	if rec := postWithCookie(t, logout, "/api/v1/auth/logout", allowedWebOrigin, stale); rec.Code != http.StatusNoContent {
		t.Fatalf("logging out with the older cookie: status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	if after := postWithCookie(t, refresh, "/api/v1/auth/refresh", allowedWebOrigin, current); after.Code != http.StatusUnauthorized {
		t.Errorf("the session's current cookie survived a logout made with an older cookie from the same session: status = %d, want %d",
			after.Code, http.StatusUnauthorized)
	}
}

// TestLogout_ChecksTheOriginFirst: a cross-site page that can log a user out
// can sign them out of their shelter mid-task on every visit. Less severe than
// a forged refresh, and exactly as cheap to prevent.
func TestLogout_ChecksTheOriginFirst(t *testing.T) {
	env := dbtest.Postgres(t)
	refresh, logout := sessionHandlers(t, env)

	cookie := loginForCookie(t, env, "logout-origin@refugio.test")

	if rec := postWithCookie(t, logout, "/api/v1/auth/logout", evilOrigin, cookie); rec.Code != http.StatusForbidden {
		t.Fatalf("a cross-site logout: status = %d, want %d", rec.Code, http.StatusForbidden)
	}

	if after := postWithCookie(t, refresh, "/api/v1/auth/refresh", allowedWebOrigin, cookie); after.Code != http.StatusOK {
		t.Errorf("the cross-site logout revoked the session anyway: refresh status = %d, want %d", after.Code, http.StatusOK)
	}
}

// TestLogout_AnswersTheSameWithOrWithoutAValidSession keeps logout from being
// an oracle over which cookies are live, and keeps it idempotent: a client that
// logs out twice, or logs out with an expired cookie, is doing nothing wrong.
func TestLogout_AnswersTheSameWithOrWithoutAValidSession(t *testing.T) {
	env := dbtest.Postgres(t)
	_, logout := sessionHandlers(t, env)

	valid := loginForCookie(t, env, "logout-idempotent@refugio.test")

	responses := map[string]*httptest.ResponseRecorder{
		"a live session": postWithCookie(t, logout, "/api/v1/auth/logout", allowedWebOrigin, valid),
		"the same again": postWithCookie(t, logout, "/api/v1/auth/logout", allowedWebOrigin, valid),
		"no cookie":      postWithCookie(t, logout, "/api/v1/auth/logout", allowedWebOrigin, ""),
		"garbage":        postWithCookie(t, logout, "/api/v1/auth/logout", allowedWebOrigin, "garbage"),
	}

	for name, rec := range responses {
		if rec.Code != http.StatusNoContent {
			t.Errorf("%s: status = %d, want %d", name, rec.Code, http.StatusNoContent)
		}
		if rec.Body.Len() != 0 {
			t.Errorf("%s: body = %q, want empty", name, rec.Body.String())
		}
	}
}
