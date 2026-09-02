package httpapi

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// forwardedForHeader is the only forwarding header this package reads.
// X-Real-IP and True-Client-IP are deliberately ignored: nothing in the
// deployment target writes them, so honouring them would only widen the
// spoofing surface.
const forwardedForHeader = "X-Forwarded-For"

type contextKey int

const clientIPContextKey contextKey = iota

// ClientIPResolver derives the originating client address of a request from
// its peer address and X-Forwarded-For chain.
//
// It replaces chi's middleware.RealIP, which was removed for being spoofable:
// RealIP rewrites r.RemoteAddr from the leftmost forwarded entry regardless of
// who the peer is, so any client can forge its own address
// (GHSA-3fxj-6jh8-hvhx, GHSA-rjr7-jggh-pgcp, GHSA-9g5q-2w5x-hmxf). Per-IP rate
// limiting on login and magic-link endpoints, and the audit log's ip_hash, both
// depend on this value being non-forgeable.
//
// The algorithm is "rightmost untrusted entry", and its safety rests on one
// property of forwarding proxies: each hop APPENDS the address it observed to
// the right of the chain. Anything a client forges therefore lands to the LEFT
// of what our own infrastructure appended, and scanning from the right stops on
// the appended value before ever reaching the forged prefix.
//
//  1. Parse the TCP peer from r.RemoteAddr. If it cannot be parsed, no client
//     address can be resolved.
//  2. If the peer is not itself a trusted proxy, the request did not arrive
//     through our infrastructure. Return the peer and ignore the header
//     entirely — it is unvalidated client input.
//  3. Otherwise walk the chain right to left, skipping trusted proxy hops, and
//     return the first untrusted address.
//  4. If the chain is absent, empty, fully trusted, or contains a malformed
//     entry, return the peer.
//
// Step 4 fails closed on purpose. The peer address is always the real TCP
// counterpart and can never be forged; falling back to it degrades accuracy,
// never safety. Skipping a malformed entry instead of stopping would let an
// attacker inject garbage to push the scan leftwards into forged territory.
//
// The trusted set comes from TRUSTED_PROXIES and defaults to EMPTY, which makes
// the resolver return the peer for every request. That is the correct default:
// an unconfigured deployment gets a non-forgeable (if less precise) address
// rather than an attacker-controlled one. The set must be populated with the
// actual front-end ranges at deploy time before per-IP rate limiting is
// meaningful.
type ClientIPResolver struct {
	trusted []netip.Prefix
}

// NewClientIPResolver returns a resolver that treats addresses inside the given
// prefixes as infrastructure proxies. A nil or empty set means no proxy is
// trusted, and every request resolves to its peer address.
func NewClientIPResolver(trusted []netip.Prefix) *ClientIPResolver {
	return &ClientIPResolver{trusted: trusted}
}

// ClientIP returns the originating client address, or the zero Addr when the
// request's peer address cannot be parsed.
func (res *ClientIPResolver) ClientIP(r *http.Request) netip.Addr {
	peer, ok := parseClientAddr(hostOnly(r.RemoteAddr))
	if !ok {
		return netip.Addr{}
	}
	if !res.isTrusted(peer) {
		return peer
	}

	chain := forwardedChain(r.Header.Values(forwardedForHeader))
	for i := len(chain) - 1; i >= 0; i-- {
		addr, ok := parseClientAddr(chain[i])
		if !ok {
			return peer
		}
		if !res.isTrusted(addr) {
			return addr
		}
	}
	return peer
}

// Middleware returns chi-compatible middleware that resolves the client address
// once per request and stores it in the request context, where rate limiting
// and audit logging can read it via ClientIPFromContext.
func (res *ClientIPResolver) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if addr := res.ClientIP(r); addr.IsValid() {
				r = r.WithContext(context.WithValue(r.Context(), clientIPContextKey, addr))
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (res *ClientIPResolver) isTrusted(addr netip.Addr) bool {
	for _, prefix := range res.trusted {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// ClientIPFromContext returns the address resolved by the client IP middleware.
// The second return value is false when the middleware did not run or could not
// resolve an address, in which case callers must not fall back to reading
// r.RemoteAddr or any forwarding header themselves.
func ClientIPFromContext(ctx context.Context) (netip.Addr, bool) {
	addr, ok := ctx.Value(clientIPContextKey).(netip.Addr)
	return addr, ok && addr.IsValid()
}

// hostOnly strips the port from a "host:port" address. RemoteAddr normally
// carries one, but a bare address is accepted rather than rejected.
func hostOnly(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return remoteAddr
}

// parseClientAddr parses a single address in its canonical comparable form:
// IPv6 zones removed and IPv4-mapped IPv6 unmapped, so that prefix membership
// tests behave the same however the address was written. Entries carrying a
// port are rejected — X-Forwarded-For entries are bare addresses, and accepting
// both forms would make IPv6 parsing ambiguous.
func parseClientAddr(raw string) (netip.Addr, bool) {
	addr, err := netip.ParseAddr(strings.TrimSpace(raw))
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.WithZone("").Unmap(), true
}

// forwardedChain flattens possibly repeated X-Forwarded-For headers into one
// ordered list of entries. Reading only the first header would let an attacker
// evade the scan by splitting the chain across several header lines.
func forwardedChain(values []string) []string {
	var chain []string
	for _, value := range values {
		for _, entry := range strings.Split(value, ",") {
			if entry = strings.TrimSpace(entry); entry != "" {
				chain = append(chain, entry)
			}
		}
	}
	return chain
}
