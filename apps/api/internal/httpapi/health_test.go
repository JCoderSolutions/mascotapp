package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apihttp "github.com/gentleman/mascotapp/apps/api/internal/httpapi"
)

func TestLivenessHandler_AlwaysReturnsOK(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	apihttp.LivenessHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	assertJSONContentType(t, rec)

	var body map[string]string
	decodeJSON(t, rec, &body)
	if body["status"] != "ok" {
		t.Errorf("body[status] = %q, want %q", body["status"], "ok")
	}
}

func TestReadinessHandler_NoChecksReturnsOK(t *testing.T) {
	handler := apihttp.NewReadinessHandler(time.Second)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	assertJSONContentType(t, rec)

	var body map[string]string
	decodeJSON(t, rec, &body)
	if body["status"] != "ready" {
		t.Errorf("body[status] = %q, want %q", body["status"], "ready")
	}
}

func TestReadinessHandler_PassingCheckReturnsOK(t *testing.T) {
	handler := apihttp.NewReadinessHandler(time.Second, apihttp.ReadinessCheck{
		Name:  "database",
		Check: func(context.Context) error { return nil },
	})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestReadinessHandler_FailingCheckReturns503WithFailedName(t *testing.T) {
	handler := apihttp.NewReadinessHandler(
		time.Second,
		apihttp.ReadinessCheck{Name: "database", Check: func(context.Context) error { return nil }},
		apihttp.ReadinessCheck{Name: "storage", Check: func(context.Context) error { return errors.New("connection refused") }},
	)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	assertJSONContentType(t, rec)

	var body struct {
		Status string   `json:"status"`
		Failed []string `json:"failed"`
	}
	decodeJSON(t, rec, &body)
	if body.Status != "not ready" {
		t.Errorf("body.Status = %q, want %q", body.Status, "not ready")
	}
	if len(body.Failed) != 1 || body.Failed[0] != "storage" {
		t.Errorf("body.Failed = %v, want [storage]", body.Failed)
	}
}

func assertJSONContentType(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	got := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(got, "application/json") {
		t.Errorf("Content-Type = %q, want prefix %q", got, "application/json")
	}
}

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(target); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
}

func TestReadinessCheckTimeout(t *testing.T) {
	t.Parallel()

	// A dependency that never answers. With Neon suspending after five minutes
	// of inactivity (ADR-0003), a hanging check is the expected case, not an
	// exotic one: without a per-check deadline it would hang /readyz entirely.
	hanging := apihttp.ReadinessCheck{
		Name: "hanging",
		Check: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}

	handler := apihttp.NewReadinessHandler(50*time.Millisecond, hanging)

	rec := httptest.NewRecorder()
	start := time.Now()
	handler(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	elapsed := time.Since(start)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if elapsed > time.Second {
		t.Fatalf("handler took %s, want it to give up near the 50ms deadline", elapsed)
	}
	if !strings.Contains(rec.Body.String(), "hanging") {
		t.Fatalf("body = %q, want it to name the failing check", rec.Body.String())
	}
}

func TestReadinessChecksRunConcurrently(t *testing.T) {
	t.Parallel()

	// Three checks that each sleep 100ms. Run in series that is 300ms; run
	// concurrently it is ~100ms. A slow dependency must not be paid three times.
	slow := func(name string) apihttp.ReadinessCheck {
		return apihttp.ReadinessCheck{
			Name: name,
			Check: func(ctx context.Context) error {
				select {
				case <-time.After(100 * time.Millisecond):
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			},
		}
	}

	handler := apihttp.NewReadinessHandler(time.Second, slow("a"), slow("b"), slow("c"))

	rec := httptest.NewRecorder()
	start := time.Now()
	handler(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	elapsed := time.Since(start)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("took %s, want concurrent execution near 100ms", elapsed)
	}
}
