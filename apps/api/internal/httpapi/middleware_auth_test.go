package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gentleman/mascotapp/apps/api/internal/auth"
	"github.com/gentleman/mascotapp/apps/api/internal/httpapi"
)

// The tenant boundary (authorization-rbac / *Tenant scope derives only from the
// verified token claim*, §5.3, ADR-0002 and ADR-0008).
//
// This is the success criterion of Phase 02 and the single place where the
// whole multi-tenancy story can be undone by one line of Go. RLS is the last
// line of defence, not the only one: it answers "which rows may this scope
// see", and it answers correctly for whatever scope it is handed. Handing it
// the wrong shelter produces a perfectly correct evaluation against the wrong
// tenant, with no error raised anywhere.
//
// So the property under test is not "the right rows come back". It is: the
// value that reaches `WithTenant` came from the verified claim and from
// **nothing else**.
//
// Every refusal below is asserted twice — the status the client sees, and the
// fact that the scoper and the handler were **never reached**. A 403 written
// after `WithTenant` already opened a transaction on the attacker's shelter is
// not a refusal; it is a leak with an apologetic status code.

const (
	testJWTSecret  = "an-integration-test-secret-of-at-least-32-bytes"
	testJWTIssuer  = "mascotapp-test"
	testJWTAudienc = "mascotapp-api-test"
)

// -----------------------------------------------------------------------------
// Spies. Both fail the test on contact, so "never invoked" is the default and
// reaching them has to be opted into by the test that expects it.
// -----------------------------------------------------------------------------

// stubTx satisfies pgx.Tx by embedding it. No method is ever called — the
// handler only checks that a transaction arrived — and a nil embedded interface
// panics loudly if that assumption ever stops holding.
type stubTx struct{ pgx.Tx }

// scoperSpy records what the middleware handed to WithTenant.
type scoperSpy struct {
	t *testing.T

	// allow must be set for the scoper to run fn. Left false, any call fails
	// the test, which is how the refusal scenarios assert "never invoked".
	allow bool

	called    bool
	gotID     uuid.UUID
	callCount int
}

func (s *scoperSpy) scope(ctx context.Context, shelterID uuid.UUID, fn func(context.Context, pgx.Tx) error) error {
	s.t.Helper()

	s.called = true
	s.callCount++
	s.gotID = shelterID

	if !s.allow {
		s.t.Errorf("WithTenant was invoked with shelter %s on a request that must be refused "+
			"before any transaction opens", shelterID)
		return nil
	}

	return fn(ctx, stubTx{})
}

// handlerSpy is the protected handler. Same rule: being reached is the
// exception, not the default.
type handlerSpy struct {
	t *testing.T

	allow bool

	called bool
	claims auth.AccessClaims
	hasTx  bool
}

func (h *handlerSpy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.t.Helper()

	h.called = true
	if !h.allow {
		h.t.Error("the protected handler ran on a request that must be refused before it")
	}

	h.claims, _ = httpapi.ClaimsFromContext(r.Context())
	_, h.hasTx = httpapi.TxFromContext(r.Context())

	w.WriteHeader(http.StatusOK)
}

// -----------------------------------------------------------------------------
// Fixtures.
// -----------------------------------------------------------------------------

func newTestIssuer(t *testing.T) *auth.TokenIssuer {
	t.Helper()

	issuer, err := auth.NewTokenIssuer([]byte(testJWTSecret), testJWTIssuer, testJWTAudienc)
	if err != nil {
		t.Fatalf("building the test issuer: %v", err)
	}

	return issuer
}

// tokenFor mints a real token with a real signature. Nothing here is faked:
// a stubbed verifier would let this whole file stay green over a middleware
// that never verifies anything.
func tokenFor(t *testing.T, shelterID *uuid.UUID, now time.Time) string {
	t.Helper()

	raw, err := newTestIssuer(t).Issue(auth.AccessClaims{
		Subject:   uuid.New(),
		ShelterID: shelterID,
		Role:      "admin",
		AMR:       []string{"pwd", "otp"},
	}, now)
	if err != nil {
		t.Fatalf("issuing a test token: %v", err)
	}

	return raw
}

// tenantRequest wires the real middleware chain onto a tenant-scoped route and
// runs one request through it.
//
// The route carries the shelter in its path exactly as a real one would. That
// is the point: the path segment must be routing, never authority.
func tenantRequest(
	t *testing.T,
	scoper *scoperSpy,
	handler *handlerSpy,
	target string,
	decorate func(*http.Request),
	now time.Time,
) *httptest.ResponseRecorder {
	t.Helper()

	router := chi.NewRouter()
	router.Route("/api/v1/shelters/{"+httpapi.ShelterIDPathParam+"}", func(r chi.Router) {
		r.Use(httpapi.RequireAuth(newTestIssuer(t), func() time.Time { return now }))
		r.Use(httpapi.RequireTenant(scoper.scope))
		r.Get("/pets", handler.ServeHTTP)
	})

	req := httptest.NewRequest(http.MethodGet, target, nil)
	if decorate != nil {
		decorate(req)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	return rec
}

func bearer(token string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }
}

// -----------------------------------------------------------------------------
// Scenario: a path shelter_id matching the claim proceeds.
// -----------------------------------------------------------------------------

func TestRequireTenant_APathShelterIDMatchingTheClaimProceeds(t *testing.T) {
	shelterA := uuid.New()
	now := time.Now()

	scoper := &scoperSpy{t: t, allow: true}
	handler := &handlerSpy{t: t, allow: true}

	rec := tenantRequest(t, scoper, handler,
		"/api/v1/shelters/"+shelterA.String()+"/pets",
		bearer(tokenFor(t, &shelterA, now)), now)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	if !scoper.called {
		t.Fatal("WithTenant was never invoked, so the request ran outside tenant scope entirely")
	}
	if scoper.gotID != shelterA {
		t.Errorf("WithTenant got shelter %s, want the claim's %s", scoper.gotID, shelterA)
	}
	if !handler.called {
		t.Error("the handler never ran")
	}
	if handler.claims.ShelterID == nil || *handler.claims.ShelterID != shelterA {
		t.Errorf("the handler read claims %+v, want shelter %s in context", handler.claims, shelterA)
	}
	if !handler.hasTx {
		t.Error("no transaction reached the handler; it would have to open its own, unscoped")
	}
}

// The test above cannot tell a middleware that read the claim from one that
// parsed the path, because on a matching request both produce the same uuid.
// This one can: the route carries **no shelter segment at all**, so the only
// place the scope can have come from is the claim.
//
// It is the positive half of the boundary. Without it, an implementation that
// took its scope from the path would pass every other test in this file.
func TestRequireTenant_ScopeComesFromTheClaimOnARouteWithNoShelterInThePath(t *testing.T) {
	shelterA := uuid.New()
	now := time.Now()

	scoper := &scoperSpy{t: t, allow: true}
	handler := &handlerSpy{t: t, allow: true}

	router := chi.NewRouter()
	router.Route("/api/v1/me", func(r chi.Router) {
		r.Use(httpapi.RequireAuth(newTestIssuer(t), func() time.Time { return now }))
		r.Use(httpapi.RequireTenant(scoper.scope))
		r.Get("/pets", handler.ServeHTTP)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/pets", nil)
	bearer(tokenFor(t, &shelterA, now))(req)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. Body: %s", rec.Code, rec.Body.String())
	}
	if scoper.callCount != 1 {
		t.Fatalf("WithTenant was invoked %d times, want exactly 1", scoper.callCount)
	}
	if scoper.gotID != shelterA {
		t.Errorf("WithTenant got %s, want the claim's %s. With no shelter in the path, the "+
			"claim is the only source there is", scoper.gotID, shelterA)
	}
}

// -----------------------------------------------------------------------------
// Scenario: a forged shelter_id in the path is refused.
// -----------------------------------------------------------------------------

func TestRequireTenant_AForgedPathShelterIDIsRefusedBeforeAnythingRuns(t *testing.T) {
	shelterA := uuid.New()
	shelterB := uuid.New()
	now := time.Now()

	scoper := &scoperSpy{t: t} // allow:false — any call fails the test
	handler := &handlerSpy{t: t}

	rec := tenantRequest(t, scoper, handler,
		"/api/v1/shelters/"+shelterB.String()+"/pets",
		bearer(tokenFor(t, &shelterA, now)), now)

	// 403, not 404 and not a silent rescope to A. Quietly preferring the claim
	// would make a probe for shelter B look exactly like success, and would
	// make a client bug undetectable from either side.
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403. A mismatch answered with anything else is either a "+
			"leak or an oracle. Body: %s", rec.Code, rec.Body.String())
	}
	if scoper.called {
		t.Errorf("WithTenant opened a transaction (shelter %s) on a forged request", scoper.gotID)
	}
	if handler.called {
		t.Error("the handler ran on a forged request")
	}
}

// A shelter named in a query string or a header is not authority either. The
// spec names all three locations; the path is only the most obvious one.
func TestRequireTenant_AShelterIDDisagreeingInAQueryOrHeaderIsRefused(t *testing.T) {
	shelterA := uuid.New()
	shelterB := uuid.New()
	now := time.Now()

	for name, decorate := range map[string]func(*http.Request){
		"query": func(r *http.Request) {
			q := r.URL.Query()
			q.Set("shelter_id", shelterB.String())
			r.URL.RawQuery = q.Encode()
		},
		"header": func(r *http.Request) { r.Header.Set("X-Shelter-ID", shelterB.String()) },
	} {
		t.Run(name, func(t *testing.T) {
			scoper := &scoperSpy{t: t}
			handler := &handlerSpy{t: t}

			token := tokenFor(t, &shelterA, now)
			rec := tenantRequest(t, scoper, handler,
				"/api/v1/shelters/"+shelterA.String()+"/pets",
				func(r *http.Request) {
					bearer(token)(r)
					decorate(r)
				}, now)

			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403 for a %s naming a different shelter", rec.Code, name)
			}
			if scoper.called {
				t.Errorf("WithTenant opened a transaction on shelter %s", scoper.gotID)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Scenario: a missing or invalid claim is refused before scope comparison.
// -----------------------------------------------------------------------------

func TestRequireAuth_AMissingOrInvalidTokenIsRefusedAndWithTenantIsNeverInvoked(t *testing.T) {
	shelterA := uuid.New()
	now := time.Now()

	// A token signed by a different issuer with a different secret: a forgery
	// that is well-formed in every way except the one that matters.
	forger, err := auth.NewTokenIssuer([]byte("a-completely-different-secret-of-32-bytes"), testJWTIssuer, testJWTAudienc)
	if err != nil {
		t.Fatalf("building the forging issuer: %v", err)
	}
	forged, err := forger.Issue(auth.AccessClaims{
		Subject: uuid.New(), ShelterID: &shelterA, Role: "owner",
	}, now)
	if err != nil {
		t.Fatalf("issuing the forged token: %v", err)
	}

	for name, tc := range map[string]struct {
		decorate func(*http.Request)
		want     int
	}{
		"no_authorization_header": {nil, http.StatusUnauthorized},
		"empty_bearer":            {bearer(""), http.StatusUnauthorized},
		"not_a_jwt":               {bearer("not-a-token"), http.StatusUnauthorized},
		"wrong_signature":         {bearer(forged), http.StatusUnauthorized},
		"expired": {
			bearer(tokenFor(t, &shelterA, now.Add(-2*auth.AccessTokenLifetime))),
			http.StatusUnauthorized,
		},
		"wrong_scheme": {
			func(r *http.Request) {
				r.Header.Set("Authorization", "Basic "+tokenFor(t, &shelterA, now))
			},
			http.StatusUnauthorized,
		},
	} {
		t.Run(name, func(t *testing.T) {
			scoper := &scoperSpy{t: t}
			handler := &handlerSpy{t: t}

			rec := tenantRequest(t, scoper, handler,
				"/api/v1/shelters/"+shelterA.String()+"/pets", tc.decorate, now)

			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d", rec.Code, tc.want)
			}
			if scoper.called {
				t.Errorf("WithTenant was invoked with %s on an unauthenticated request", scoper.gotID)
			}
		})
	}
}

// A token that verifies perfectly but carries no shelter is authenticated and
// unscoped. It is refused with 403, not 401 — the credential is fine, the
// authority is missing — and it must never reach WithTenant, which would
// otherwise be handed uuid.Nil and refuse it one layer too late.
func TestRequireTenant_AValidTokenWithNoShelterClaimIsRefusedBeforeScopeComparison(t *testing.T) {
	shelterA := uuid.New()
	now := time.Now()

	scoper := &scoperSpy{t: t}
	handler := &handlerSpy{t: t}

	rec := tenantRequest(t, scoper, handler,
		"/api/v1/shelters/"+shelterA.String()+"/pets",
		bearer(tokenFor(t, nil, now)), now)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403. Body: %s", rec.Code, rec.Body.String())
	}
	if scoper.called {
		t.Errorf("WithTenant was invoked with %s for a token that names no shelter", scoper.gotID)
	}
	if handler.called {
		t.Error("the handler ran for a token that names no shelter")
	}
}

// The zero uuid is the specific value this boundary must not accept. It is
// non-nil in Go, so a presence check written as `claims.ShelterID != nil` after
// a decode that defaulted the field would pass it straight through.
func TestRequireTenant_TheZeroShelterUUIDIsNotAShelter(t *testing.T) {
	now := time.Now()
	zero := uuid.Nil

	scoper := &scoperSpy{t: t}
	handler := &handlerSpy{t: t}

	rec := tenantRequest(t, scoper, handler,
		"/api/v1/shelters/"+zero.String()+"/pets",
		bearer(tokenFor(t, &zero, now)), now)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a token claiming the zero uuid", rec.Code)
	}
	if scoper.called {
		t.Error("WithTenant was invoked with the zero uuid; it refuses it, but one layer too late")
	}
}

// A path segment that is not a uuid at all is refused before anything is
// compared. Parsing failure must not fall through to "no path shelter, use the
// claim" — that would turn a malformed path into a bypass of the whole check.
func TestRequireTenant_AnUnparseablePathShelterIDIsRefused(t *testing.T) {
	shelterA := uuid.New()
	now := time.Now()

	scoper := &scoperSpy{t: t}
	handler := &handlerSpy{t: t}

	rec := tenantRequest(t, scoper, handler,
		"/api/v1/shelters/not-a-uuid/pets",
		bearer(tokenFor(t, &shelterA, now)), now)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for an unparseable path shelter id", rec.Code)
	}
	if scoper.called {
		t.Errorf("WithTenant was invoked with %s despite an unparseable path segment", scoper.gotID)
	}
}
