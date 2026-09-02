package httpapi_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	apihttp "github.com/gentleman/mascotapp/apps/api/internal/httpapi"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
}

func TestNewRouter_RegistersHealthzRoute(t *testing.T) {
	router := apihttp.NewRouter(testLogger(), apihttp.NewClientIPResolver(nil))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestNewRouter_RegistersReadyzRoute(t *testing.T) {
	router := apihttp.NewRouter(testLogger(), apihttp.NewClientIPResolver(nil))

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /readyz status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestNewRouter_UnknownRouteReturns404(t *testing.T) {
	router := apihttp.NewRouter(testLogger(), apihttp.NewClientIPResolver(nil))

	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /does-not-exist status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestNewRouter_RecoversFromPanicAs500(t *testing.T) {
	router := apihttp.NewRouter(testLogger(), apihttp.NewClientIPResolver(nil))
	router.Get("/panic", func(_ http.ResponseWriter, _ *http.Request) {
		panic("boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()

	// A recovered panic must not crash the test process.
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("GET /panic status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestNewRouter_LogsRequestsAsStructuredJSON(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	router := apihttp.NewRouter(logger, apihttp.NewClientIPResolver(nil))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	logged := buf.String()
	for _, field := range []string{`"method":"GET"`, `"path":"/healthz"`, `"status":200`, "duration", "request_id"} {
		if !bytes.Contains(buf.Bytes(), []byte(field)) {
			t.Errorf("log output missing %q; got: %s", field, logged)
		}
	}
}

func TestNewRouter_ResolvesClientIPForHandlers(t *testing.T) {
	resolver := apihttp.NewClientIPResolver(mustPrefixes(t, "10.0.0.0/8"))
	router := apihttp.NewRouter(testLogger(), resolver)

	var (
		got string
		ok  bool
	)
	router.Get("/whoami", func(_ http.ResponseWriter, r *http.Request) {
		var addr netip.Addr
		addr, ok = apihttp.ClientIPFromContext(r.Context())
		got = addr.String()
	})

	req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
	req.RemoteAddr = "10.0.0.1:443"
	req.Header.Set("X-Forwarded-For", "1.1.1.1, 203.0.113.9")
	router.ServeHTTP(httptest.NewRecorder(), req)

	if !ok {
		t.Fatal("handler saw no client IP; the resolver middleware is not wired into the router")
	}
	if got != "203.0.113.9" {
		t.Fatalf("handler saw client IP %s, want 203.0.113.9", got)
	}
}
