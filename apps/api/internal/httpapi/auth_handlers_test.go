package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gentleman/mascotapp/apps/api/internal/db"
	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
	"github.com/gentleman/mascotapp/apps/api/internal/db/sqlcgen"
	"github.com/gentleman/mascotapp/apps/api/internal/httpapi"
)

// Registration is the one flow in this phase that spans two database roles, so
// it is the one place where "did the write land" cannot be answered by the
// handler's own response. Every assertion below reads the rows back through the
// role that owns them.
//
// These run against a container because the properties under test are database
// properties: an idempotent insert on a citext UNIQUE, a column default the
// caller cannot override, and two transactions whose failure mode is defined by
// what the first one leaves behind.

func TestRegister_LandsTheShelterPendingVerification(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := httpapi.RegisterHandler(env.AuthPool, env.TenantPool)

	body := `{
		"email": "founder@refugio.test",
		"password": "a-long-enough-password",
		"full_name": "Ana Founder",
		"shelter_name": "Refugio Esperanza",
		"shelter_slug": "refugio-esperanza"
	}`
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body)))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got struct {
		ShelterID string `json:"shelter_id"`
		UserID    string `json:"user_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding the response: %v", err)
	}

	shelterID, err := uuid.Parse(got.ShelterID)
	if err != nil {
		t.Fatalf("shelter_id %q is not a uuid: %v", got.ShelterID, err)
	}

	status := shelterStatus(t, env, shelterID)
	if status != "pending_verification" {
		t.Errorf("shelter status = %q, want %q -- LT-2: a shelter cannot publish before a human verifies it", status, "pending_verification")
	}
}

// TestRegister_IgnoresAStatusSubmittedByTheCaller is the spec's own wording, and
// the interesting part is which layer answers it.
//
// TWO layers refuse independently. The handler does not read a `status` field
// off the request at all, and `app_tenant` holds no INSERT privilege on
// `shelters.status` (00015), so a statement that named it would be refused by
// the grant before any policy ran. This test pins the OUTCOME; the grant is
// pinned in the migration suite. Neither one alone would tell you the field was
// dropped rather than written and then overwritten.
func TestRegister_IgnoresAStatusSubmittedByTheCaller(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := httpapi.RegisterHandler(env.AuthPool, env.TenantPool)

	body := `{
		"email": "sneaky@refugio.test",
		"password": "a-long-enough-password",
		"full_name": "Sneaky Founder",
		"shelter_name": "Refugio Atajo",
		"shelter_slug": "refugio-atajo",
		"status": "verified"
	}`
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body)))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got struct {
		ShelterID string `json:"shelter_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding the response: %v", err)
	}
	shelterID := uuid.MustParse(got.ShelterID)

	if status := shelterStatus(t, env, shelterID); status != "pending_verification" {
		t.Errorf("shelter status = %q after the caller submitted \"verified\", want %q", status, "pending_verification")
	}
}

// TestRegister_CreatesTheFounderMembershipAtomically pins the second half of
// P2-D6's second transaction. A shelter whose founder holds no membership is a
// shelter nobody can administer, and it is invisible: the shelter row looks
// perfectly fine on its own.
func TestRegister_CreatesTheFounderMembershipAtomically(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := httpapi.RegisterHandler(env.AuthPool, env.TenantPool)

	body := `{
		"email": "owner@refugio.test",
		"password": "a-long-enough-password",
		"full_name": "Owner Founder",
		"shelter_name": "Refugio Duena",
		"shelter_slug": "refugio-duena"
	}`
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body)))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got struct {
		ShelterID string `json:"shelter_id"`
		UserID    string `json:"user_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding the response: %v", err)
	}

	role, status := founderMembership(t, env, uuid.MustParse(got.ShelterID), uuid.MustParse(got.UserID))
	if role != "owner" {
		t.Errorf("founder role = %q, want %q", role, "owner")
	}
	if status != "active" {
		t.Errorf("founder membership status = %q, want %q -- the founder is not invited, they are already here", status, "active")
	}
}

// TestRegister_IsIdempotentOnTheEmail asserts the compensation P2-D6 chose:
// "do nothing". If the second transaction fails, a user exists with no shelter,
// and re-running registration has to resolve it rather than collide.
//
// Registering twice with one email is the observable form of that: the second
// call must not fail on the citext UNIQUE, and must not create a second user.
// The email differs in case between the two calls on purpose -- `citext` is what
// makes them the same address, and a handler that lower-cased by hand would pass
// this while leaving the property untested.
func TestRegister_IsIdempotentOnTheEmail(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := httpapi.RegisterHandler(env.AuthPool, env.TenantPool)

	register := func(email, slug, name string) string {
		t.Helper()
		body := `{
			"email": "` + email + `",
			"password": "a-long-enough-password",
			"full_name": "Twice Founder",
			"shelter_name": "` + name + `",
			"shelter_slug": "` + slug + `"
		}`
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body)))
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
		}
		var got struct {
			UserID string `json:"user_id"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decoding the response: %v", err)
		}
		return got.UserID
	}

	first := register("twice@refugio.test", "refugio-uno", "Refugio Uno")
	second := register("TWICE@Refugio.TEST", "refugio-dos", "Refugio Dos")

	if first != second {
		t.Errorf("user_id = %q on the second registration, want %q -- the citext UNIQUE makes these one address, so the second call must find the existing user rather than create another",
			second, first)
	}
	if n := countUsersWithEmail(t, env, "twice@refugio.test"); n != 1 {
		t.Errorf("users with that email = %d, want 1", n)
	}
}

// TestRegister_RefusesInvalidInput keeps the handler from reaching the database
// with something the database would have to refuse. Each case names the field
// that is wrong, because a 400 that does not is a 400 the caller cannot act on.
func TestRegister_RefusesInvalidInput(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := httpapi.RegisterHandler(env.AuthPool, env.TenantPool)

	for _, tt := range []struct {
		name string
		body string
	}{
		{"not json at all", `not json`},
		{"no email", `{"password":"a-long-enough-password","full_name":"A","shelter_name":"S","shelter_slug":"s"}`},
		{"no password", `{"email":"a@b.test","full_name":"A","shelter_name":"S","shelter_slug":"s"}`},
		{"no shelter name", `{"email":"a@b.test","password":"a-long-enough-password","full_name":"A","shelter_slug":"s"}`},
		{"no shelter slug", `{"email":"a@b.test","password":"a-long-enough-password","full_name":"A","shelter_name":"S"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(tt.body)))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d for %s", rec.Code, http.StatusBadRequest, tt.name)
			}
		})
	}
}

// TestRegister_NeverEchoesThePassword is the anti-vacuity case for the whole
// file: every test above asserts what the handler WROTE, and none of them would
// notice the handler writing the password back out. A registration response is
// the first thing a client logs.
func TestRegister_NeverEchoesThePassword(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := httpapi.RegisterHandler(env.AuthPool, env.TenantPool)

	const password = "a-very-recognisable-password"
	body := `{
		"email": "quiet@refugio.test",
		"password": "` + password + `",
		"full_name": "Quiet Founder",
		"shelter_name": "Refugio Silencio",
		"shelter_slug": "refugio-silencio"
	}`
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body)))

	// Searched across the WHOLE response, headers included, rather than in a
	// named field: the point is that the credential does not appear anywhere,
	// and a field-scoped check only proves it is not in the field you thought of.
	if strings.Contains(rec.Body.String(), password) {
		t.Error("the response body contains the submitted password")
	}
	for name, values := range rec.Header() {
		for _, v := range values {
			if strings.Contains(v, password) {
				t.Errorf("header %s contains the submitted password", name)
			}
		}
	}
}

// TestRegister_StoresThePasswordHashedAndNotInTheClear reads the credential back
// through the auth role. A handler that stored the password verbatim would pass
// every other test in this file.
func TestRegister_StoresThePasswordHashedAndNotInTheClear(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := httpapi.RegisterHandler(env.AuthPool, env.TenantPool)

	const password = "another-recognisable-password"
	body := `{
		"email": "hashed@refugio.test",
		"password": "` + password + `",
		"full_name": "Hashed Founder",
		"shelter_name": "Refugio Hash",
		"shelter_slug": "refugio-hash"
	}`
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body)))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	stored := storedPasswordHash(t, env, "hashed@refugio.test")
	if stored == password {
		t.Fatal("the password is stored in the clear")
	}
	if !strings.HasPrefix(stored, "$argon2id$") {
		t.Errorf("stored hash = %q, want a PHC-format Argon2id encoding", stored)
	}
}

// --- readers, all of them through the role that owns the row ---

func shelterStatus(t *testing.T, env *dbtest.Env, shelterID uuid.UUID) string {
	t.Helper()

	var status string
	err := db.WithTenant(context.Background(), env.TenantPool, shelterID, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT status FROM shelters WHERE id = $1`, shelterID).Scan(&status)
	})
	if err != nil {
		t.Fatalf("reading the shelter back: %v", err)
	}
	return status
}

func founderMembership(t *testing.T, env *dbtest.Env, shelterID, userID uuid.UUID) (role, status string) {
	t.Helper()

	err := db.WithTenant(context.Background(), env.TenantPool, shelterID, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT role, status FROM memberships WHERE user_id = $1`, userID,
		).Scan(&role, &status)
	})
	if err != nil {
		t.Fatalf("reading the founder's membership back: %v", err)
	}
	return role, status
}

func countUsersWithEmail(t *testing.T, env *dbtest.Env, email string) int {
	t.Helper()

	var n int
	err := db.WithAuthLookup(context.Background(), env.AuthPool, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE email = $1`, email).Scan(&n)
	})
	if err != nil {
		t.Fatalf("counting users: %v", err)
	}
	return n
}

func storedPasswordHash(t *testing.T, env *dbtest.Env, email string) string {
	t.Helper()

	var hash string
	err := db.WithAuthLookup(context.Background(), env.AuthPool, func(ctx context.Context, tx pgx.Tx) error {
		row, err := sqlcgen.New(tx).GetUserCredentialsByEmail(ctx, email)
		if err != nil {
			return err
		}
		hash = row.PasswordHash.String
		return nil
	})
	if err != nil {
		t.Fatalf("reading the stored credential: %v", err)
	}
	return hash
}
