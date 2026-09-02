package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"sync"
	"time"
)

// ReadinessCheck is a named dependency check used by the readiness
// handler. Checks are injected so callers (e.g. Postgres, R2) can be
// registered without this package depending on their implementations.
type ReadinessCheck struct {
	Name  string
	Check func(context.Context) error
}

// LivenessHandler reports whether the process is alive. It never depends
// on external systems and always returns 200.
func LivenessHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// NewReadinessHandler builds a readiness handler that runs every given check
// and reports 200 when all pass, or 503 naming the failed checks when any fail.
// With zero checks it always returns 200.
//
// Checks run CONCURRENTLY under a shared per-request deadline. Both properties
// matter in production: Neon suspends after five minutes idle (ADR-0003), so a
// dependency that hangs is the expected case, not an exotic one. Serial checks
// would pay every slow dependency's latency in turn, and without a deadline a
// single hung check would hang /readyz forever — turning a readiness probe into
// the outage it was meant to report.
func NewReadinessHandler(timeout time.Duration, checks ...ReadinessCheck) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		var (
			mu     sync.Mutex
			failed []string
			wg     sync.WaitGroup
		)

		for _, c := range checks {
			wg.Add(1)
			go func(c ReadinessCheck) {
				defer wg.Done()
				if err := c.Check(ctx); err != nil {
					mu.Lock()
					failed = append(failed, c.Name)
					mu.Unlock()
				}
			}(c)
		}
		wg.Wait()

		if len(failed) > 0 {
			sort.Strings(failed) // stable output: easier to assert and to read in logs
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"status": "not ready",
				"failed": failed,
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
