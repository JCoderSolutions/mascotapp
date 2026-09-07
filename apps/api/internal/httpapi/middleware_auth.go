package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gentleman/mascotapp/apps/api/internal/auth"
)

// The tenant boundary (authorization-rbac / *Tenant scope derives only from the
// verified token claim*, §5.3, ADR-0002 and ADR-0008).
//
// This is the success criterion of Phase 02. RLS is the last line of defence,
// not the only one, and it is not an identity check: it answers "which rows may
// this scope see" correctly for whatever scope it is handed. Hand it the wrong
// shelter and every policy evaluates perfectly against the wrong tenant, with no
// error raised anywhere. So the guarantee this file owes is narrower and harder
// than "the right rows come back":
//
//	the value that reaches WithTenant came from the verified claim, and from
//	nothing else.

// ShelterIDPathParam is the chi route parameter a tenant-scoped path carries.
//
// It exists for ROUTING and for the mismatch check below — never as a source of
// scope. A URL segment is client-controlled input; treating it as authority is
// the whole vulnerability this middleware exists to prevent.
const ShelterIDPathParam = "shelterID"

// The other two places a client can name a shelter. Both are checked for the
// same reason the path is: not because either is trusted, but because a request
// that names a shelter other than its own is refused rather than quietly
// rescoped.
const (
	shelterIDQueryParam = "shelter_id"
	shelterIDHeader     = "X-Shelter-ID"
)

// authContextKey is deliberately its own type rather than reusing contextKey
// from clientip.go. Two iota blocks over one type silently share values, and
// the collision would hand one middleware's value to another's reader.
type authContextKey int

const (
	claimsContextKey authContextKey = iota
	txContextKey
)

// TenantScoper opens a tenant-scoped transaction and runs fn inside it.
//
// It is the shape of `db.WithTenant` with the pool already closed over, and the
// seam exists for two reasons. It keeps `internal/httpapi` from importing a
// pool, and it lets the tests assert the one thing that matters here — that on
// a refused request this function is NEVER CALLED. A boundary that opens a
// transaction on the attacker's shelter and then writes 403 has already failed;
// the status code is an apology, not a refusal.
type TenantScoper func(ctx context.Context, shelterID uuid.UUID, fn func(context.Context, pgx.Tx) error) error

// RequireAuth verifies the bearer token and puts the claims in the request
// context. It sets no tenant scope: that is RequireTenant's job, and splitting
// them is what lets an unscoped route (choosing a shelter, reading your own
// profile) authenticate without inventing a tenant.
//
// now is injected so tests can place a request at a chosen instant rather than
// sleeping through a token lifetime.
func RequireAuth(issuer *auth.TokenIssuer, now func() time.Time) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				unauthorized(w)
				return
			}

			claims, err := issuer.Verify(raw, now())
			if err != nil {
				// The error is discarded on purpose. Every verification failure
				// is one 401 with one body: telling a caller WHICH check failed
				// tells an attacker how close their forgery got.
				unauthorized(w)
				return
			}

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), claimsContextKey, claims)))
		})
	}
}

// RequireTenant resolves tenant scope from the verified claim and opens the
// transaction the handler runs in.
//
// The order of the checks below is part of the contract, not an implementation
// detail:
//
//  1. authenticated at all             -> 500, this is a wiring bug
//  2. the claim names a real shelter    -> 403
//  3. nothing in the request contradicts it -> 403
//  4. only then, WithTenant
//
// Every refusal happens before step 4, so a refused request never reaches the
// database at all.
func RequireTenant(scope TenantScoper) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok {
				// RequireTenant mounted without RequireAuth above it. Answering
				// 403 would hide a wiring mistake behind a plausible-looking
				// refusal on every request; 500 says the server is wrong, which
				// it is.
				http.Error(w, "server misconfigured", http.StatusInternalServerError)
				return
			}

			// Both halves are load-bearing. uuid.Nil is NOT nil in Go, so a
			// presence check alone lets the zero uuid through to WithTenant --
			// which refuses it, one layer too late and as a 500 instead of the
			// 403 this is.
			if claims.ShelterID == nil || *claims.ShelterID == uuid.Nil {
				forbidden(w)
				return
			}
			shelterID := *claims.ShelterID

			if !requestAgreesOnShelter(r, shelterID) {
				forbidden(w)
				return
			}

			err := scope(r.Context(), shelterID, func(ctx context.Context, tx pgx.Tx) error {
				next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, txContextKey, tx)))
				return nil
			})
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		})
	}
}

// requestAgreesOnShelter reports whether every shelter identifier the client
// supplied matches the claim.
//
// An absent identifier agrees: most routes name no shelter, and the claim is
// the source either way. A present one must parse AND match — an unparseable
// value is a disagreement, never a reason to fall through to "no identifier
// supplied", which would turn a malformed path into a bypass of this whole
// check.
func requestAgreesOnShelter(r *http.Request, claimed uuid.UUID) bool {
	supplied := []string{
		chi.URLParam(r, ShelterIDPathParam),
		r.URL.Query().Get(shelterIDQueryParam),
		r.Header.Get(shelterIDHeader),
	}

	for _, raw := range supplied {
		if raw == "" {
			continue
		}
		parsed, err := uuid.Parse(raw)
		if err != nil || parsed != claimed {
			return false
		}
	}

	return true
}

// bearerToken extracts the credential from an Authorization header value.
//
// The scheme is compared case-insensitively because RFC 7235 defines it that
// way; the credential is not trimmed or repaired, because a token that needs
// repairing is not a token this service issued.
func bearerToken(header string) (string, bool) {
	scheme, credential, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || credential == "" {
		return "", false
	}

	return credential, true
}

// unauthorized answers a request that failed to authenticate. The
// WWW-Authenticate header is RFC 7235's requirement for a 401 and tells a
// well-behaved client to present a token rather than retry blindly.
func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

// forbidden answers an authenticated request that may not have this scope.
//
// 403 rather than 404: hiding the resource would make this boundary an
// existence oracle only for callers who guess right, while still leaking to
// those who guess wrong. And rather than 401, because the credential is fine --
// it is the authority that is missing.
func forbidden(w http.ResponseWriter) {
	http.Error(w, "forbidden", http.StatusForbidden)
}

// ClaimsFromContext returns the claims RequireAuth verified.
//
// ok is false when RequireAuth did not run. Callers must treat that as "not
// authenticated" and never fall back to reading a token themselves: a second
// verification path is a second place for the rules to drift.
func ClaimsFromContext(ctx context.Context) (auth.AccessClaims, bool) {
	claims, ok := ctx.Value(claimsContextKey).(auth.AccessClaims)

	return claims, ok
}

// TxFromContext returns the tenant-scoped transaction RequireTenant opened.
//
// ok is false outside a tenant-scoped route. A handler that finds none must
// fail rather than open its own: a transaction opened elsewhere carries no
// app.shelter_id, so every RLS policy matches nothing and the handler reads the
// empty result as "no data" instead of as an error.
func TxFromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txContextKey).(pgx.Tx)

	return tx, ok
}
