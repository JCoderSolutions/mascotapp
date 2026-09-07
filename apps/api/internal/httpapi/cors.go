package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// CORS and the origin allowlist (identity-and-session / *The refresh cookie and
// cross-origin requests are protected for separate origins*, design P2-D9).
//
// Separate origins are decided (proposal Decision 4), so the web app is always
// cross-origin to this API and the allowlist is load-bearing rather than
// ceremonial. It is ONE list, shared with csrf.go on purpose: P2-D9 chose
// `Origin` verification over a double-submit token precisely because the
// allowlist CORS already needs is also the answer to "did this come from us".
// Two lists would be two things to keep in sync, and the drift would be silent.

// corsMaxAge is how long a browser may cache a preflight result. Ten minutes:
// long enough that the preflight is not paid per request, short enough that a
// policy change takes effect within a coffee break.
const corsMaxAge = "600"

// allowedRequestHeaders is what a browser may send on a cross-origin request.
// An explicit list rather than reflecting Access-Control-Request-Headers back:
// reflecting turns the client into the author of the policy.
const allowedRequestHeaders = "Authorization, Content-Type"

// allowedMethods is the verb list echoed on a preflight. It describes the API
// as a whole; per-route method checks stay with the router, which is the only
// thing that knows them.
const allowedMethods = "GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS"

// OriginAllowlist is the set of origins allowed to make credentialed requests.
//
// It is a type rather than a []string so the validation below happens exactly
// once, at construction, and so no caller can compare against a raw slice with
// its own idea of what "matches" means.
type OriginAllowlist struct {
	allowed map[string]struct{}
}

// NewOriginAllowlist validates and builds the allowlist from configured origins
// (`WEB_ORIGINS`).
//
// Every rejection here is a configuration error: true or false at boot,
// independent of any request. Refusing at construction makes a misconfiguration
// a startup failure someone must look at, rather than a runtime behaviour
// nobody notices until the wrong thing is allowed.
func NewOriginAllowlist(origins []string) (*OriginAllowlist, error) {
	if len(origins) == 0 {
		return nil, errors.New("httpapi: the origin allowlist is empty; WEB_ORIGINS is " +
			"unset or unparsed, and every browser request would be refused for a reason " +
			"nobody will trace back to configuration")
	}

	allowed := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		if err := validOrigin(origin); err != nil {
			return nil, err
		}
		allowed[origin] = struct{}{}
	}

	return &OriginAllowlist{allowed: allowed}, nil
}

// validOrigin reports whether raw is a serialized origin this API may trust.
//
// An origin is scheme + host + optional port and NOTHING else. Anything with a
// path, a query, or userinfo is not an origin, and a browser will never send it
// — so an entry shaped like that never matches anything and is a typo that
// would silently disable one allowlist row.
func validOrigin(raw string) error {
	switch raw {
	case "":
		return errors.New("httpapi: an empty string is not an origin")
	case "*":
		return errors.New("httpapi: `*` is not an allowed origin. The wildcard is illegal " +
			"together with credentials, so it does not widen access -- it silently disables it")
	case "null":
		return errors.New("httpapi: `null` is not an allowed origin. It is what a sandboxed " +
			"iframe and some redirects send, so allowlisting it hands this boundary to anyone " +
			"who can get a sandboxed frame to run")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("httpapi: %q is not a valid origin: %w", raw, err)
	}
	if parsed.Scheme != "https" {
		// http is refused rather than merely discouraged: the refresh cookie is
		// `Secure`, so an http origin cannot carry a session anyway, and
		// allowing one only creates a downgrade path that looks configured.
		return fmt.Errorf("httpapi: %q is not an https origin", raw)
	}
	if parsed.Host == "" {
		return fmt.Errorf("httpapi: %q has no host", raw)
	}
	if parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return fmt.Errorf("httpapi: %q is not a bare origin; an origin is scheme, host and "+
			"optional port, with no path, query or userinfo -- a browser never sends one of "+
			"these, so this entry would match nothing", raw)
	}

	return nil
}

// Allows reports whether origin is on the allowlist.
//
// Exact string equality on the serialized origin, deliberately. Every cheaper
// comparison is a vulnerability with a friendly name: `HasSuffix` matches
// `https://mascotapp.test.evil.example`, `Contains` matches anything, and
// trimming or normalising invents a match the browser never sent.
func (a *OriginAllowlist) Allows(origin string) bool {
	_, ok := a.allowed[origin]

	return ok
}

// CORS answers preflights and writes the cross-origin headers, refusing any
// request that names an origin the allowlist does not contain.
//
// # Refusing server-side is stronger than CORS, on purpose
//
// Standard CORS is advisory: the server describes a policy and the BROWSER
// enforces it. A non-browser client ignores the headers entirely. Refusing here
// turns the description into a control, which is what the spec asks for --
// "CORS refuses the request before it reaches the handler".
//
// A request with NO Origin is not a CORS request and passes through untouched.
// Refusing those would break server-to-server callers, health probes and curl,
// and it is not what protects state changes: RequireTrustedOrigin is.
func CORS(allowlist *OriginAllowlist) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Vary before anything else, including on the refusal and on the
			// no-Origin path. A cache keying this response without it may serve
			// one origin's Access-Control-Allow-Origin header to another, which
			// turns a correct allowlist into a wrong one at the cache layer.
			w.Header().Add("Vary", "Origin")

			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			if !allowlist.Allows(origin) {
				// No CORS headers on the way out. A refusal that still echoed
				// the origin would be describing a permission it just denied.
				forbidden(w)
				return
			}

			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")

			// A preflight is the CORS negotiation, not a route. Answering it
			// here keeps OPTIONS from reaching handlers that never expect it.
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Set("Access-Control-Allow-Methods", allowedMethods)
				w.Header().Set("Access-Control-Allow-Headers", allowedRequestHeaders)
				w.Header().Set("Access-Control-Max-Age", corsMaxAge)
				w.WriteHeader(http.StatusNoContent)

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// isStateChanging reports whether the method may change server state.
//
// GET, HEAD and OPTIONS are the safe methods of RFC 9110 §9.2.1. They are not
// gated by the CSRF check: the public catalogue is anonymous and cross-origin
// by design, and gating reads would break it for no gain.
func isStateChanging(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}
