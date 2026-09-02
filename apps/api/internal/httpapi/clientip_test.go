package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	apihttp "github.com/gentleman/mascotapp/apps/api/internal/httpapi"
)

func mustPrefixes(t *testing.T, raw ...string) []netip.Prefix {
	t.Helper()

	out := make([]netip.Prefix, 0, len(raw))
	for _, s := range raw {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			t.Fatalf("bad test fixture prefix %q: %v", s, err)
		}
		out = append(out, p)
	}
	return out
}

// TestClientIPResolver_ClientIP covers the whole resolution contract. The
// spoofing cases are the reason this type exists: chi's middleware.RealIP was
// removed because it trusts X-Forwarded-For unconditionally.
func TestClientIPResolver_ClientIP(t *testing.T) {
	tests := []struct {
		name       string
		trusted    []string
		remoteAddr string
		headers    map[string][]string
		want       string // empty means "no client IP could be resolved"
	}{
		{
			name:       "no trusted proxies configured ignores the forwarded header",
			trusted:    nil,
			remoteAddr: "198.51.100.7:5555",
			headers:    map[string][]string{"X-Forwarded-For": {"203.0.113.9"}},
			want:       "198.51.100.7",
		},
		{
			name:       "untrusted peer ignores the forwarded header",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "198.51.100.7:5555",
			headers:    map[string][]string{"X-Forwarded-For": {"203.0.113.9"}},
			want:       "198.51.100.7",
		},
		{
			name:       "trusted peer takes the single forwarded entry",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "10.0.0.1:443",
			headers:    map[string][]string{"X-Forwarded-For": {"203.0.113.9"}},
			want:       "203.0.113.9",
		},
		{
			name:       "a forged prefix cannot outrank the appended client address",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "10.0.0.1:443",
			headers:    map[string][]string{"X-Forwarded-For": {"1.1.1.1, 203.0.113.9"}},
			want:       "203.0.113.9",
		},
		{
			name:       "forged trusted-looking hops cannot push the scan further left",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "10.0.0.1:443",
			headers:    map[string][]string{"X-Forwarded-For": {"9.9.9.9, 10.0.0.99, 203.0.113.9"}},
			want:       "203.0.113.9",
		},
		{
			name:       "skips trailing trusted proxy hops",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "10.0.0.1:443",
			headers:    map[string][]string{"X-Forwarded-For": {"203.0.113.9, 10.0.0.5, 10.0.0.1"}},
			want:       "203.0.113.9",
		},
		{
			name:       "repeated forwarded headers are joined in order",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "10.0.0.1:443",
			headers:    map[string][]string{"X-Forwarded-For": {"1.1.1.1", "203.0.113.9"}},
			want:       "203.0.113.9",
		},
		{
			name:       "tolerates whitespace around entries",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "10.0.0.1:443",
			headers:    map[string][]string{"X-Forwarded-For": {"  1.1.1.1 ,   203.0.113.9  "}},
			want:       "203.0.113.9",
		},
		{
			name:       "a malformed entry stops the scan and falls back to the peer",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "10.0.0.1:443",
			headers:    map[string][]string{"X-Forwarded-For": {"not-an-ip"}},
			want:       "10.0.0.1",
		},
		{
			name:       "an entry carrying a port is treated as malformed",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "10.0.0.1:443",
			headers:    map[string][]string{"X-Forwarded-For": {"203.0.113.9:1234"}},
			want:       "10.0.0.1",
		},
		{
			// The malformed entry must STOP the scan, not be skipped. Skipping
			// would let a caller inside a trusted range use garbage as a wall
			// and have the address it forged to the left returned as the client.
			name:       "a malformed entry is not skipped over to reach forged entries behind it",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "10.0.0.1:443",
			headers:    map[string][]string{"X-Forwarded-For": {"1.1.1.1, not-an-ip, 10.0.0.5"}},
			want:       "10.0.0.1",
		},
		{
			name:       "an empty forwarded header falls back to the peer",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "10.0.0.1:443",
			headers:    map[string][]string{"X-Forwarded-For": {""}},
			want:       "10.0.0.1",
		},
		{
			name:       "a fully trusted chain falls back to the peer",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "10.0.0.1:443",
			headers:    map[string][]string{"X-Forwarded-For": {"10.0.0.5, 10.0.0.6"}},
			want:       "10.0.0.1",
		},
		{
			name:       "unmaps ipv4-mapped ipv6 entries",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "10.0.0.1:443",
			headers:    map[string][]string{"X-Forwarded-For": {"::ffff:203.0.113.9"}},
			want:       "203.0.113.9",
		},
		{
			name:       "strips the zone from ipv6 entries",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "10.0.0.1:443",
			headers:    map[string][]string{"X-Forwarded-For": {"2001:db8::1%eth0"}},
			want:       "2001:db8::1",
		},
		{
			name:       "accepts a bracketed ipv6 peer",
			trusted:    []string{"2001:db8::/32"},
			remoteAddr: "[2001:db8::1]:443",
			headers:    map[string][]string{"X-Forwarded-For": {"203.0.113.9"}},
			want:       "203.0.113.9",
		},
		{
			name:       "accepts a peer address with no port",
			trusted:    nil,
			remoteAddr: "198.51.100.7",
			want:       "198.51.100.7",
		},
		{
			name:       "an unparseable peer resolves to nothing",
			trusted:    nil,
			remoteAddr: "not-an-address",
			want:       "",
		},
		{
			name:       "ignores x-real-ip",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "10.0.0.1:443",
			headers:    map[string][]string{"X-Real-Ip": {"203.0.113.9"}},
			want:       "10.0.0.1",
		},
		{
			name:       "ignores true-client-ip",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "10.0.0.1:443",
			headers:    map[string][]string{"True-Client-Ip": {"203.0.113.9"}},
			want:       "10.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := apihttp.NewClientIPResolver(mustPrefixes(t, tt.trusted...))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			for name, values := range tt.headers {
				for _, v := range values {
					req.Header.Add(name, v)
				}
			}

			got := resolver.ClientIP(req)

			if tt.want == "" {
				if got.IsValid() {
					t.Fatalf("ClientIP() = %s, want an invalid address", got)
				}
				return
			}
			if got.String() != tt.want {
				t.Fatalf("ClientIP() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestClientIPResolver_MiddlewareStoresAddressInContext(t *testing.T) {
	resolver := apihttp.NewClientIPResolver(mustPrefixes(t, "10.0.0.0/8"))

	var (
		got netip.Addr
		ok  bool
	)
	handler := resolver.Middleware()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, ok = apihttp.ClientIPFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:443"
	req.Header.Set("X-Forwarded-For", "1.1.1.1, 203.0.113.9")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if !ok {
		t.Fatal("ClientIPFromContext() reported no address; want one")
	}
	if got.String() != "203.0.113.9" {
		t.Fatalf("ClientIPFromContext() = %s, want 203.0.113.9", got)
	}
}

func TestClientIPResolver_MiddlewareReportsNoAddressWhenPeerIsUnparseable(t *testing.T) {
	resolver := apihttp.NewClientIPResolver(nil)

	var ok bool
	handler := resolver.Middleware()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, ok = apihttp.ClientIPFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "not-an-address"
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if ok {
		t.Fatal("ClientIPFromContext() reported an address for an unparseable peer; want none")
	}
}

func TestClientIPFromContext_ReportsNothingWithoutMiddleware(t *testing.T) {
	if _, ok := apihttp.ClientIPFromContext(context.Background()); ok {
		t.Fatal("ClientIPFromContext() reported an address on a bare context; want none")
	}
}
