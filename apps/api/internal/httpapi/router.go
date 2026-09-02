// Package httpapi wires the HTTP transport: routing, middleware, and the
// health/readiness endpoints. It lives in internal/http and depends only
// on the standard library and chi.
package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// readinessTimeout caps how long /readyz waits on its dependency checks.
// Kept short on purpose: a probe that takes longer than the caller's own
// timeout is useless to whoever is asking.
const readinessTimeout = 2 * time.Second

// NewRouter builds the chi router with the standard middleware stack
// (request ID, panic recovery, client IP resolution, structured request
// logging) and the health/readiness routes registered.
//
// clientIP must not be nil. chi's middleware.RealIP is deliberately NOT
// used in its place: it is deprecated and spoofable, rewriting r.RemoteAddr
// from the leftmost X-Forwarded-For value whether or not the infrastructure
// actually sets it (GHSA-3fxj-6jh8-hvhx, GHSA-rjr7-jggh-pgcp,
// GHSA-9g5q-2w5x-hmxf). See ClientIPResolver for what replaces it.
//
// The request logger does NOT record the resolved address: IP addresses are
// personal data and are only persisted as a salted hash in the audit log.
func NewRouter(logger *slog.Logger, clientIP *ClientIPResolver) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(clientIP.Middleware())
	r.Use(requestLogger(logger))

	r.Get("/healthz", LivenessHandler)
	r.Get("/readyz", NewReadinessHandler(readinessTimeout))

	return r
}

// requestLogger returns chi middleware that logs each request as JSON via
// slog, including method, path, status, duration, and request id. It
// replaces chi's default text logger.
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			logger.Info("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"duration", time.Since(start).String(),
				"request_id", middleware.GetReqID(r.Context()),
			)
		})
	}
}
