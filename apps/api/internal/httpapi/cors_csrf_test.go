package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gentleman/mascotapp/apps/api/internal/httpapi"
)

// CORS and CSRF for separate origins (identity-and-session / *The refresh
// cookie and cross-origin requests are protected for separate origins*, and
// design P2-D9).
//
// # What "CSRF token" means here, because the spec and the design disagree
//
// The spec requirement was written asking for "a valid CSRF token". P2-D9
// afterwards REJECTED the double-submit token and replaced it with
// `Origin`/`Sec-Fetch-Site` verification: the browser sets `Origin` on
// cross-site requests and JavaScript cannot forge it, the allowlist is the one
// CORS already uses, and there is nothing to store, rotate, or reset on a cold
// start. The task text carries the same translation. So the third scenario's
// "missing or invalid CSRF token" is tested here as "missing or disagreeing
// `Origin`/`Sec-Fetch-Site`". That is the design superseding the spec's
// mechanism, not a scenario going untested — and it is written down rather than
// left for a reviewer to infer.
//
// # The trap this file exists to pin
//
// Separate origins are decided (proposal Decision 4). So EVERY legitimate
// browser request from the web app carries `Sec-Fetch-Site: cross-site`. A CSRF
// check written the obvious way — "refuse cross-site" — refuses one hundred
// percent of real traffic while a reviewer nods along, because that rule sounds
// exactly like what a CSRF defence should say.
//
// `Origin` is the control. `Sec-Fetch-Site` is only the fallback for requests
// that legitimately carry no `Origin` at all.

const (
	allowedWebOrigin = "https://app.mascotapp.test"
	otherAllowed     = "https://admin.mascotapp.test"
	evilOrigin       = "https://mascotapp.test.evil.example"
)

// okHandler is the protected handler. It records being reached, which is how
// every refusal below asserts that the refusal happened BEFORE the handler.
type okHandler struct{ reached bool }

func (h *okHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	h.reached = true
	w.WriteHeader(http.StatusOK)
}

func testAllowlist(t *testing.T) *httpapi.OriginAllowlist {
	t.Helper()

	list, err := httpapi.NewOriginAllowlist([]string{allowedWebOrigin, otherAllowed})
	if err != nil {
		t.Fatalf("building the allowlist: %v", err)
	}

	return list
}

// send runs one request through the middleware under test.
func send(t *testing.T, mw func(http.Handler) http.Handler, method, target string, headers map[string]string) (*httptest.ResponseRecorder, *okHandler) {
	t.Helper()

	handler := &okHandler{}
	req := httptest.NewRequest(method, target, nil)
	for name, value := range headers {
		req.Header.Set(name, value)
	}

	rec := httptest.NewRecorder()
	mw(handler).ServeHTTP(rec, req)

	return rec, handler
}

// -----------------------------------------------------------------------------
// The allowlist itself. Configuration errors are refused at construction, where
// they cannot be missed, rather than at the first request where they are a log
// line nobody reads.
// -----------------------------------------------------------------------------

func TestNewOriginAllowlist_RefusesTheWildcardAndOtherNonOrigins(t *testing.T) {
	for name, entry := range map[string]string{
		// Illegal with credentials anyway; refusing it here makes the
		// misconfiguration fail loudly at boot instead of silently at runtime.
		"wildcard": "*",
		// `Origin: null` is what a sandboxed iframe and some redirects send.
		// Allowlisting it hands the whole boundary to anyone who can get a
		// sandboxed frame to run.
		"null":             "null",
		"empty":            "",
		"no_scheme":        "app.mascotapp.test",
		"plain_http":       "http://app.mascotapp.test",
		"with_path":        "https://app.mascotapp.test/app",
		"with_query":       "https://app.mascotapp.test?x=1",
		"trailing_slash":   "https://app.mascotapp.test/",
		"with_userinfo":    "https://user@app.mascotapp.test",
		"no_host":          "https://",
		"not_a_url_at_all": "::::",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := httpapi.NewOriginAllowlist([]string{entry}); err == nil {
				t.Errorf("%q was accepted as an allowed origin", entry)
			}
		})
	}
}

func TestNewOriginAllowlist_RefusesAnEmptyList(t *testing.T) {
	// An empty allowlist is not "allow nothing" in practice: it is a
	// misconfigured WEB_ORIGINS, and a server that starts with one serves an
	// app whose every request fails for reasons nobody will connect to config.
	if _, err := httpapi.NewOriginAllowlist(nil); err == nil {
		t.Error("an empty allowlist was accepted")
	}
}

// -----------------------------------------------------------------------------
// Scenario: an allowlisted origin with valid credentials succeeds.
// -----------------------------------------------------------------------------

func TestCORS_AnAllowlistedOriginIsEchoedExactlyAndNeverAsAWildcard(t *testing.T) {
	rec, handler := send(t, httpapi.CORS(testAllowlist(t)), http.MethodPost, "/api/v1/auth/refresh",
		map[string]string{"Origin": allowedWebOrigin})

	if rec.Code != http.StatusOK || !handler.reached {
		t.Fatalf("status = %d, handler reached = %v; want 200 and reached", rec.Code, handler.reached)
	}

	got := rec.Header().Get("Access-Control-Allow-Origin")
	if got == "*" {
		t.Fatal("Access-Control-Allow-Origin is `*`. With credentials the browser rejects it, so " +
			"the whole allowlist silently stops working -- and if a browser ever honoured it, " +
			"every origin on the internet would read authenticated responses")
	}
	if got != allowedWebOrigin {
		t.Errorf("Access-Control-Allow-Origin = %q, want the exact origin %q", got, allowedWebOrigin)
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Error("Access-Control-Allow-Credentials is not `true`; the refresh cookie would never be sent")
	}
}

// Vary: Origin is not a nicety. Without it, any cache between the API and the
// browser may serve one origin's Access-Control-Allow-Origin header to another,
// which turns a correct allowlist into a wrong one at the cache layer.
func TestCORS_VariesOnOrigin(t *testing.T) {
	rec, _ := send(t, httpapi.CORS(testAllowlist(t)), http.MethodGet, "/api/v1/pets",
		map[string]string{"Origin": allowedWebOrigin})

	if vary := rec.Header().Get("Vary"); vary == "" {
		t.Error("no Vary header; a shared cache may serve one origin's CORS headers to another")
	}
}

// -----------------------------------------------------------------------------
// Scenario: a non-allowlisted origin is refused regardless of credentials.
// -----------------------------------------------------------------------------

func TestCORS_ANonAllowlistedOriginIsRefusedBeforeTheHandlerEvenWithCredentials(t *testing.T) {
	for name, origin := range map[string]string{
		// A suffix that contains the real domain. String containment, or a
		// naive HasSuffix, matches this; equality does not.
		"lookalike_suffix": evilOrigin,
		"null":             "null",
		"scheme_downgrade": "http://app.mascotapp.test",
		"port_added":       "https://app.mascotapp.test:8443",
		"subdomain_added":  "https://evil.app.mascotapp.test",
	} {
		t.Run(name, func(t *testing.T) {
			rec, handler := send(t, httpapi.CORS(testAllowlist(t)), http.MethodPost, "/api/v1/auth/refresh",
				map[string]string{"Origin": origin, "Cookie": "__Secure-mascotapp_refresh=abc.def"})

			if handler.reached {
				t.Errorf("the handler ran for origin %q", origin)
			}
			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403 for origin %q", rec.Code, origin)
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
				t.Errorf("Access-Control-Allow-Origin = %q on a refused origin", got)
			}
		})
	}
}

// Standard CORS is advisory: the server describes a policy and the BROWSER
// enforces it. A non-browser client ignores the headers entirely. Refusing
// server-side is what turns this from a description into a control, and the
// spec asks for exactly that -- "CORS refuses the request before it reaches the
// handler".
func TestCORS_APreflightFromADisallowedOriginIsRefusedAndNeverReachesTheHandler(t *testing.T) {
	rec, handler := send(t, httpapi.CORS(testAllowlist(t)), http.MethodOptions, "/api/v1/auth/refresh",
		map[string]string{
			"Origin":                        evilOrigin,
			"Access-Control-Request-Method": http.MethodPost,
		})

	if handler.reached {
		t.Error("a preflight reached the handler")
	}
	if rec.Code == http.StatusOK || rec.Code == http.StatusNoContent {
		t.Errorf("status = %d; a preflight from a disallowed origin was answered as if allowed", rec.Code)
	}
}

func TestCORS_AnAllowedPreflightIsAnsweredWithoutReachingTheHandler(t *testing.T) {
	rec, handler := send(t, httpapi.CORS(testAllowlist(t)), http.MethodOptions, "/api/v1/auth/refresh",
		map[string]string{
			"Origin":                        allowedWebOrigin,
			"Access-Control-Request-Method": http.MethodPost,
		})

	if handler.reached {
		t.Error("a preflight reached the handler; OPTIONS is the CORS negotiation, not a route")
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != allowedWebOrigin {
		t.Error("the preflight did not echo the allowed origin")
	}
	if rec.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("no Access-Control-Allow-Methods on a preflight response")
	}
}

// A request with no Origin at all is not a CORS request. Refusing it here would
// break server-to-server callers, health probes and curl, and it is NOT what
// protects state changes -- RequireTrustedOrigin below is.
func TestCORS_ARequestWithNoOriginPassesThroughWithNoCORSHeaders(t *testing.T) {
	rec, handler := send(t, httpapi.CORS(testAllowlist(t)), http.MethodGet, "/api/v1/pets", nil)

	if !handler.reached {
		t.Error("a request with no Origin was refused; that is not a CORS decision to make")
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("CORS headers were written for a request that is not cross-origin")
	}
}

// -----------------------------------------------------------------------------
// Scenario: a missing or disagreeing Origin/Sec-Fetch-Site is refused even from
// an allowlisted origin (the spec's "missing or invalid CSRF token", per P2-D9).
// -----------------------------------------------------------------------------

// THE test of this file. With separate origins every legitimate browser request
// carries `Sec-Fetch-Site: cross-site`. A check written as "refuse cross-site"
// refuses one hundred percent of real traffic — and reads, to a reviewer, like
// exactly what a CSRF defence should say.
func TestRequireTrustedOrigin_AnAllowlistedOriginSucceedsEvenWhenSecFetchSiteSaysCrossSite(t *testing.T) {
	rec, handler := send(t, httpapi.RequireTrustedOrigin(testAllowlist(t)), http.MethodPost, "/api/v1/auth/refresh",
		map[string]string{
			"Origin":         allowedWebOrigin,
			"Sec-Fetch-Site": "cross-site",
			"Sec-Fetch-Mode": "cors",
			"Cookie":         "__Secure-mascotapp_refresh=abc.def",
		})

	if rec.Code != http.StatusOK || !handler.reached {
		t.Fatalf("status = %d, reached = %v. Separate origins are decided (Decision 4), so "+
			"`cross-site` is what EVERY legitimate request from the web app carries. Refusing it "+
			"refuses all real traffic while looking like a correct CSRF rule",
			rec.Code, handler.reached)
	}
}

func TestRequireTrustedOrigin_AStateChangingRequestWithNeitherHeaderIsRefused(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			rec, handler := send(t, httpapi.RequireTrustedOrigin(testAllowlist(t)), method, "/api/v1/auth/refresh",
				map[string]string{"Cookie": "__Secure-mascotapp_refresh=abc.def"})

			if handler.reached {
				t.Errorf("%s with neither Origin nor Sec-Fetch-Site reached the handler", method)
			}
			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403. Fail closed is the decision (P2-D9): a "+
					"state-changing request that proves nothing about where it came from is refused",
					rec.Code)
			}
		})
	}
}

func TestRequireTrustedOrigin_ADisallowedOriginIsRefusedOnAStateChangingRequest(t *testing.T) {
	rec, handler := send(t, httpapi.RequireTrustedOrigin(testAllowlist(t)), http.MethodPost, "/api/v1/auth/refresh",
		map[string]string{"Origin": evilOrigin, "Sec-Fetch-Site": "cross-site"})

	if handler.reached {
		t.Error("a request from a disallowed origin reached the handler")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

// Origin is the control; Sec-Fetch-Site is the fallback for requests that
// legitimately carry no Origin. `same-origin` is the only value that proves the
// request was not triggered from another site.
func TestRequireTrustedOrigin_WithNoOriginOnlySecFetchSiteSameOriginIsAccepted(t *testing.T) {
	for value, wantOK := range map[string]bool{
		"same-origin": true,
		"same-site":   false,
		"cross-site":  false,
		// A user-initiated navigation. It cannot be a form post from another
		// site, but it also proves nothing about the caller, and fail-closed
		// means the benefit of the doubt goes the other way.
		"none":    false,
		"garbage": false,
	} {
		t.Run(value, func(t *testing.T) {
			rec, handler := send(t, httpapi.RequireTrustedOrigin(testAllowlist(t)), http.MethodPost, "/api/v1/auth/refresh",
				map[string]string{"Sec-Fetch-Site": value})

			if handler.reached != wantOK {
				t.Errorf("Sec-Fetch-Site: %s -> handler reached = %v, want %v", value, handler.reached, wantOK)
			}
			if wantOK && rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rec.Code)
			}
			if !wantOK && rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403", rec.Code)
			}
		})
	}
}

// A disallowed Origin is refused even when Sec-Fetch-Site says same-origin.
// The two are not alternatives to be tried until one passes: a present Origin
// is the answer, and a contradicting Sec-Fetch-Site does not rescue it.
func TestRequireTrustedOrigin_APresentOriginIsNotRescuedByAFriendlySecFetchSite(t *testing.T) {
	rec, handler := send(t, httpapi.RequireTrustedOrigin(testAllowlist(t)), http.MethodPost, "/api/v1/auth/refresh",
		map[string]string{"Origin": evilOrigin, "Sec-Fetch-Site": "same-origin"})

	if handler.reached {
		t.Error("a disallowed Origin was rescued by Sec-Fetch-Site: same-origin, which any " +
			"non-browser client can set to anything it likes")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

// Safe methods are not state-changing and are not gated here. Gating them would
// break the public catalogue, which is anonymous and cross-origin by design.
func TestRequireTrustedOrigin_SafeMethodsPassWithNoHeadersAtAll(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			_, handler := send(t, httpapi.RequireTrustedOrigin(testAllowlist(t)), method, "/api/v1/pets", nil)

			if !handler.reached {
				t.Errorf("%s was refused; safe methods change no state", method)
			}
		})
	}
}
