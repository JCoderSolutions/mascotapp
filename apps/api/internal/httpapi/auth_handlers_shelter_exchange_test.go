package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gentleman/mascotapp/apps/api/internal/auth"
	"github.com/gentleman/mascotapp/apps/api/internal/db"
	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
	"github.com/gentleman/mascotapp/apps/api/internal/httpapi"
)

// POST /auth/session/shelter is the MINT POINT for the shelter_id claim
// RequireTenant later trusts (authorization-rbac / *Tenant scope derives only
// from the verified token claim*). What this file pins is narrower than "the
// right token comes back": it is that the claim traces to exactly one check —
// an ACTIVE membership — and nothing else.
//
// No membership at all, an invited one, and a revoked one are three distinct
// unauthorized states, not one combined "not active" case. T-02-031
// (obs-a13da1b17fdd6562) is why each gets its own test here rather than one
// table-driven case for "anything but active": a mutant that breaks the
// no-membership check but leaves invited/revoked refused (or vice versa)
// would hide behind a single combined assertion.

// seedActiveMembership registers a shelter, which mints its founder as an
// active 'owner' membership, and returns both ids the exchange needs.
// registerOwner (login_totp_test.go) only returns the user id; this file
// needs the shelter id too, so it posts to RegisterHandler directly rather
// than widen a signature other tests already depend on.
func seedActiveMembership(t *testing.T, env *dbtest.Env, email, slug string) (userID, shelterID uuid.UUID) {
	t.Helper()

	body := `{
		"email": "` + email + `",
		"password": "a-long-enough-password",
		"full_name": "Exchange Fixture",
		"shelter_name": "Refugio ` + slug + `",
		"shelter_slug": "` + slug + `"
	}`
	rec := httptest.NewRecorder()
	httpapi.RegisterHandler(env.AuthPool, env.TenantPool).
		ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("registering %s: status = %d: %s", email, rec.Code, rec.Body.String())
	}

	var got struct {
		UserID    string `json:"user_id"`
		ShelterID string `json:"shelter_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding the registration response: %v", err)
	}
	return uuid.MustParse(got.UserID), uuid.MustParse(got.ShelterID)
}

// seedMembershipWithStatus inserts a user with a membership row in shelterID
// carrying the given status directly, so 'invited' and 'revoked' can be
// produced without going through InviteMember's hard-coded 'invited'.
func seedMembershipWithStatus(t *testing.T, env *dbtest.Env, shelterID uuid.UUID, email, role, status string) uuid.UUID {
	t.Helper()

	userID := seedAdopter(t, env, email, "a-long-enough-password")

	err := db.WithTenant(context.Background(), env.TenantPool, shelterID, func(ctx context.Context, tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx,
			`INSERT INTO memberships (id, user_id, shelter_id, role, status) VALUES ($1, $2, $3, $4, $5)`,
			uuid.New(), userID, shelterID, role, status)
		return execErr
	})
	if err != nil {
		t.Fatalf("seeding a %s membership: %v", status, err)
	}
	return userID
}

// sessionTokenFor mints the unscoped token a real login would hand this
// caller: no shelter_id, RoleUnscoped, amr [pwd]. The exchange handler is what
// turns this into a shelter-scoped one.
func sessionTokenFor(t *testing.T, issuer *auth.TokenIssuer, userID uuid.UUID, now time.Time) string {
	t.Helper()

	raw, err := issuer.Issue(auth.AccessClaims{
		Subject: userID,
		Role:    httpapi.RoleUnscoped,
		AMR:     []string{"pwd"},
	}, now)
	if err != nil {
		t.Fatalf("issuing a session token: %v", err)
	}
	return raw
}

func shelterExchangeHandler(t *testing.T, env *dbtest.Env, issuer *auth.TokenIssuer) http.Handler {
	t.Helper()
	return httpapi.RequireAuth(issuer, time.Now)(httpapi.ShelterExchangeHandler(env.AuthPool, issuer, time.Now))
}

func postShelterExchange(t *testing.T, handler http.Handler, token, shelterID string) *httptest.ResponseRecorder {
	t.Helper()

	body := `{"shelter_id":"` + shelterID + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session/shelter", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestShelterExchange_AnActiveMembershipIssuesATokenScopedToThatShelter(t *testing.T) {
	env := dbtest.Postgres(t)
	issuer := newTestIssuer(t)
	handler := shelterExchangeHandler(t, env, issuer)

	userID, shelterID := seedActiveMembership(t, env, "exchange-active@refugio.test", "refugio-exchange-active")
	token := sessionTokenFor(t, issuer, userID, time.Now())

	rec := postShelterExchange(t, handler, token, shelterID.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding the response: %v", err)
	}

	claims, err := issuer.Verify(got.AccessToken, time.Now())
	if err != nil {
		t.Fatalf("the issued token does not verify: %v", err)
	}
	if claims.ShelterID == nil || *claims.ShelterID != shelterID {
		t.Errorf("shelter_id = %v, want %s", claims.ShelterID, shelterID)
	}
	if claims.Role != "owner" {
		t.Errorf("role = %q, want %q -- minted from the caller's active membership, not asserted by the caller", claims.Role, "owner")
	}
	if claims.Subject != userID {
		t.Errorf("subject = %s, want %s", claims.Subject, userID)
	}
}

func TestShelterExchange_NoMembershipAtAllIsRefused(t *testing.T) {
	env := dbtest.Postgres(t)
	issuer := newTestIssuer(t)
	handler := shelterExchangeHandler(t, env, issuer)

	userID := seedAdopter(t, env, "exchange-none@refugio.test", "a-long-enough-password")
	_, shelterID := seedActiveMembership(t, env, "exchange-none-owner@refugio.test", "refugio-exchange-none")
	token := sessionTokenFor(t, issuer, userID, time.Now())

	rec := postShelterExchange(t, handler, token, shelterID.String())
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d for a caller with no membership in this shelter at all: %s",
			rec.Code, http.StatusForbidden, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "access_token") {
		t.Error("a refused exchange returned a token")
	}
}

func TestShelterExchange_AnInvitedMembershipIsRefused(t *testing.T) {
	env := dbtest.Postgres(t)
	issuer := newTestIssuer(t)
	handler := shelterExchangeHandler(t, env, issuer)

	_, shelterID := seedActiveMembership(t, env, "exchange-invited-owner@refugio.test", "refugio-exchange-invited")
	userID := seedMembershipWithStatus(t, env, shelterID, "exchange-invited@refugio.test", "volunteer", "invited")
	token := sessionTokenFor(t, issuer, userID, time.Now())

	rec := postShelterExchange(t, handler, token, shelterID.String())
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d for an invitation not yet accepted: %s",
			rec.Code, http.StatusForbidden, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "access_token") {
		t.Error("a refused exchange returned a token")
	}
}

func TestShelterExchange_ARevokedMembershipIsRefused(t *testing.T) {
	env := dbtest.Postgres(t)
	issuer := newTestIssuer(t)
	handler := shelterExchangeHandler(t, env, issuer)

	_, shelterID := seedActiveMembership(t, env, "exchange-revoked-owner@refugio.test", "refugio-exchange-revoked")
	userID := seedMembershipWithStatus(t, env, shelterID, "exchange-revoked@refugio.test", "volunteer", "revoked")
	token := sessionTokenFor(t, issuer, userID, time.Now())

	rec := postShelterExchange(t, handler, token, shelterID.String())
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d for a revoked membership: %s",
			rec.Code, http.StatusForbidden, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "access_token") {
		t.Error("a refused exchange returned a token")
	}
}
