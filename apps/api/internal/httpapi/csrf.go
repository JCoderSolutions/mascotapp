package httpapi

import "net/http"

// CSRF by `Origin` / `Sec-Fetch-Site` verification (design P2-D9).
//
// # Why not a double-submit token
//
// P2-D9 rejected it. The browser sets `Origin` on cross-site requests and
// JavaScript cannot forge it, and the allowlist to check it against is the one
// CORS already needs -- one source of truth, nothing to store, nothing to
// rotate, nothing a cold start resets. Double-submit needs a second readable
// cookie, a rotation story, and a client that remembers to echo it: the same
// property, with more moving parts and one more thing to get subtly wrong.
//
// The spec requirement says "a valid CSRF token" because it was written before
// that decision. This IS the CSRF control it asks for; the mechanism changed,
// the property did not.
//
// # The mistake this file is shaped to avoid
//
// Separate origins are decided (proposal Decision 4). So every legitimate
// browser request from the web app arrives with `Sec-Fetch-Site: cross-site`.
// A check written the obvious way -- "refuse cross-site" -- refuses one hundred
// percent of real traffic, and reads to a reviewer like exactly what a CSRF
// defence should say.
//
// `Origin` is the control. `Sec-Fetch-Site` is only the fallback for a request
// that legitimately carries no `Origin`.

// secFetchSiteHeader is set by the browser and, like Origin, cannot be set by
// page JavaScript -- it is a forbidden header name.
const secFetchSiteHeader = "Sec-Fetch-Site"

// RequireTrustedOrigin refuses a state-changing request that cannot show it
// came from an allowed origin.
//
// The decision, in order:
//
//  1. safe method            -> pass; it changes nothing
//  2. Origin present         -> it decides, alone. On the allowlist, pass;
//     otherwise refuse
//  3. no Origin, Sec-Fetch-Site: same-origin -> pass
//  4. anything else          -> refuse (fail closed, P2-D9)
//
// Step 2 is exhaustive on purpose: a present `Origin` is answered by the
// allowlist and by nothing else. Falling through to `Sec-Fetch-Site` after a
// disallowed `Origin` would let a non-browser client -- which can set
// `Sec-Fetch-Site` to whatever it likes, since only browsers are bound by the
// forbidden-header rule -- rescue an origin the allowlist just rejected.
func RequireTrustedOrigin(allowlist *OriginAllowlist) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isStateChanging(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			if origin := r.Header.Get("Origin"); origin != "" {
				if !allowlist.Allows(origin) {
					forbidden(w)
					return
				}

				next.ServeHTTP(w, r)
				return
			}

			// No Origin at all. `same-origin` is the only value that proves the
			// request was not triggered from another site: a cross-site form
			// post reports `cross-site`, and `none` (a user-initiated
			// navigation) proves nothing about the caller. Fail closed means
			// the benefit of the doubt goes the other way.
			if r.Header.Get(secFetchSiteHeader) == "same-origin" {
				next.ServeHTTP(w, r)
				return
			}

			// Neither header. The honest cost of failing closed, named in
			// P2-D9: a non-browser client must send an allowlisted Origin, or
			// use the bearer path instead. Native clients keep the refresh
			// token in secure storage and send it in the body -- no cookie, so
			// no CSRF exposure at all.
			forbidden(w)
		})
	}
}
