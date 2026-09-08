package httpapi_test

import (
	"bytes"
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

// The phase's E2E success criterion lives in this file: an owner or admin
// cannot complete login without a second factor.
//
// Every test here goes through the real handler against a real database. The
// property is about what the SERVER refuses, and a test that stubbed the
// credential lookup would be asserting the stub.

// testKEK is a 32-byte envelope key. It is a fixture, not a secret: the point of
// encrypting the TOTP secret at rest is that a database dump does not hand over
// second factors, and that property is exercised by sealing here exactly as the
// enrolment path will.
var testKEK = bytes.Repeat([]byte{0x2a}, 32)

func totpLoginHandler(t *testing.T, env *dbtest.Env) http.Handler {
	t.Helper()

	issuer, err := auth.NewTokenIssuer([]byte(testJWTSecret), testJWTIssuer, testJWTAudienc)
	if err != nil {
		t.Fatalf("building the token issuer: %v", err)
	}
	envelope, err := auth.NewEnvelope(testKEK)
	if err != nil {
		t.Fatalf("building the envelope: %v", err)
	}
	return httpapi.LoginHandler(env.AuthPool, issuer, time.Now, envelope)
}

// enrolTOTP seals a secret for the user and writes it to `totp_secret_enc`.
//
// It runs through WithAuthUser as `app_auth`, which is the only role holding
// UPDATE on that column (00013). Writing it as the owner would work and would
// prove nothing: the point is that the same role the login path uses can put it
// there.
func enrolTOTP(t *testing.T, env *dbtest.Env, userID uuid.UUID) []byte {
	t.Helper()

	secret, err := auth.NewTOTPSecret()
	if err != nil {
		t.Fatalf("generating a TOTP secret: %v", err)
	}
	envelope, err := auth.NewEnvelope(testKEK)
	if err != nil {
		t.Fatalf("building the envelope: %v", err)
	}
	sealed, err := envelope.Seal(secret, userID)
	if err != nil {
		t.Fatalf("sealing the TOTP secret: %v", err)
	}

	err = db.WithAuthUser(context.Background(), env.AuthPool, userID, func(ctx context.Context, tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx, `UPDATE users SET totp_secret_enc = $1 WHERE id = $2`, sealed, userID)
		return execErr
	})
	if err != nil {
		t.Fatalf("enrolling TOTP: %v", err)
	}
	return secret
}

// currentTOTP is what an authenticator app would be showing right now.
func currentTOTP(t *testing.T, secret []byte) string {
	t.Helper()

	// Derived by brute force over the six-digit space rather than by calling an
	// unexported generator: the test asserts against the same VerifyTOTP the
	// handler uses, so it cannot pass by sharing a bug with a generator only
	// the tests can see.
	now := time.Now()
	for candidate := range 1_000_000 {
		code := padTOTP(candidate)
		if auth.VerifyTOTP(secret, code, now) {
			return code
		}
	}
	t.Fatal("no six-digit code verified against the secret, which cannot happen")
	return ""
}

func padTOTP(n int) string {
	digits := []byte("000000")
	for i := 5; i >= 0; i-- {
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return string(digits)
}

// registerOwner registers a shelter, which makes the caller its `owner`, and
// returns the user id.
func registerOwner(t *testing.T, env *dbtest.Env, email, password, slug string) uuid.UUID {
	t.Helper()

	body := `{
		"email": "` + email + `",
		"password": "` + password + `",
		"full_name": "TOTP Fixture",
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
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding the registration response: %v", err)
	}
	return uuid.MustParse(got.UserID)
}

// issueRecoveryCodes writes a fresh set for the user and returns the
// plaintexts, which is the only moment they are readable — after this they exist
// only as SHA-256 hashes, which is the property being relied on.
func issueRecoveryCodes(t *testing.T, env *dbtest.Env, userID uuid.UUID) []string {
	t.Helper()

	codes, err := auth.NewRecoveryCodes()
	if err != nil {
		t.Fatalf("generating recovery codes: %v", err)
	}

	err = db.WithAuthUser(context.Background(), env.AuthPool, userID, func(ctx context.Context, tx pgx.Tx) error {
		return auth.RegenerateRecoveryCodes(ctx, tx, userID, codes)
	})
	if err != nil {
		t.Fatalf("storing recovery codes: %v", err)
	}

	plaintexts := make([]string, 0, len(codes))
	for _, code := range codes {
		plaintexts = append(plaintexts, code.Plaintext)
	}
	return plaintexts
}

// seedAdopter creates a user with NO membership — the shape an adopter has.
// Registration always mints an owner, so this one is inserted directly.
func seedAdopter(t *testing.T, env *dbtest.Env, email, password string) uuid.UUID {
	t.Helper()

	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hashing the adopter's password: %v", err)
	}

	userID := uuid.New()
	err = db.WithAuthUser(context.Background(), env.AuthPool, userID, func(ctx context.Context, tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx,
			`INSERT INTO users (id, email, password_hash, full_name) VALUES ($1, $2, $3, $4)`,
			userID, email, hash, "Adopter Fixture")
		return execErr
	})
	if err != nil {
		t.Fatalf("seeding the adopter: %v", err)
	}
	return userID
}

// refreshCookieNameForTest is spelled out rather than imported. The constant in
// the handler is unexported, and a test that read it would assert that the
// handler agrees with itself; this asserts the name the BROWSER will see, which
// is the contract that matters.
const refreshCookieNameForTest = "__Secure-mascotapp_refresh"

// postLoginWith sends a login with whatever second-factor fields the caller
// wants, including none.
func postLoginWith(t *testing.T, handler http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body)))
	return rec
}

func accessTokenFrom(t *testing.T, rec *httptest.ResponseRecorder) auth.AccessClaims {
	t.Helper()

	var got struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding the response: %v", err)
	}
	issuer, err := auth.NewTokenIssuer([]byte(testJWTSecret), testJWTIssuer, testJWTAudienc)
	if err != nil {
		t.Fatalf("building the verifier: %v", err)
	}
	claims, err := issuer.Verify(got.AccessToken, time.Now())
	if err != nil {
		t.Fatalf("the issued token does not verify: %v", err)
	}
	return claims
}

// TestLogin_AnOwnerWithTOTPEnrolledCannotLoginWithThePasswordAlone is the
// phase's E2E success criterion, stated as narrowly as it can be: the password
// is CORRECT, and that is still not enough.
func TestLogin_AnOwnerWithTOTPEnrolledCannotLoginWithThePasswordAlone(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := totpLoginHandler(t, env)

	const email, password = "owner-notenough@refugio.test", "a-long-enough-password"
	userID := registerOwner(t, env, email, password, "owner-notenough")
	enrolTOTP(t, env, userID)

	rec := postLoginWith(t, handler, `{"email":"`+email+`","password":"`+password+`"}`)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d -- an owner completed login with a password alone: %s",
			rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	if cookie := rec.Header().Get("Set-Cookie"); cookie != "" {
		t.Errorf("a refused login set a session cookie: %s", cookie)
	}
	if strings.Contains(rec.Body.String(), "access_token") {
		t.Errorf("a refused login returned a token: %s", rec.Body.String())
	}
}

// TestLogin_AnOwnerWithAValidTOTPCodeGetsASession is the other half. Without
// it, refusing every owner would pass the test above.
func TestLogin_AnOwnerWithAValidTOTPCodeGetsASession(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := totpLoginHandler(t, env)

	const email, password = "owner-withcode@refugio.test", "a-long-enough-password"
	userID := registerOwner(t, env, email, password, "owner-withcode")
	secret := enrolTOTP(t, env, userID)

	rec := postLoginWith(t, handler,
		`{"email":"`+email+`","password":"`+password+`","totp_code":"`+currentTOTP(t, secret)+`"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if cookie := rec.Header().Get("Set-Cookie"); !strings.Contains(cookie, refreshCookieNameForTest) {
		t.Errorf("no refresh cookie on a successful two-factor login: %s", cookie)
	}
}

// TestLogin_TheAMRRecordsThatASecondFactorWasExercised is what makes the claim
// worth carrying. A route that demands a second factor reads `amr`, and it has
// to distinguish "used" from "merely enrolled".
func TestLogin_TheAMRRecordsThatASecondFactorWasExercised(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := totpLoginHandler(t, env)

	const email, password = "owner-amr@refugio.test", "a-long-enough-password"
	userID := registerOwner(t, env, email, password, "owner-amr")
	secret := enrolTOTP(t, env, userID)

	rec := postLoginWith(t, handler,
		`{"email":"`+email+`","password":"`+password+`","totp_code":"`+currentTOTP(t, secret)+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	claims := accessTokenFrom(t, rec)
	if len(claims.AMR) != 2 || claims.AMR[0] != "pwd" || claims.AMR[1] != "otp" {
		t.Errorf("amr = %v, want [pwd otp]", claims.AMR)
	}
}

// TestLogin_ARecoveryCodeCompletesLoginInPlaceOfTOTP covers the lost phone. The
// codes exist because TOTP is mandatory for owners: without them a lost device
// locks a shelter owner out permanently.
func TestLogin_ARecoveryCodeCompletesLoginInPlaceOfTOTP(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := totpLoginHandler(t, env)

	const email, password = "owner-recovery@refugio.test", "a-long-enough-password"
	userID := registerOwner(t, env, email, password, "owner-recovery")
	enrolTOTP(t, env, userID)
	codes := issueRecoveryCodes(t, env, userID)

	rec := postLoginWith(t, handler,
		`{"email":"`+email+`","password":"`+password+`","recovery_code":"`+codes[0]+`"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	claims := accessTokenFrom(t, rec)
	if len(claims.AMR) != 2 || claims.AMR[1] != "otp" {
		t.Errorf("amr = %v, want a second factor recorded -- a recovery code IS one", claims.AMR)
	}
}

// TestLogin_ARecoveryCodeIsSpentByUsingIt is the property that makes them safe
// to print on paper. A code that still works after a login is a permanent
// password with the same length as a phone number.
func TestLogin_ARecoveryCodeIsSpentByUsingIt(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := totpLoginHandler(t, env)

	const email, password = "owner-spent@refugio.test", "a-long-enough-password"
	userID := registerOwner(t, env, email, password, "owner-spent")
	enrolTOTP(t, env, userID)
	codes := issueRecoveryCodes(t, env, userID)

	body := `{"email":"` + email + `","password":"` + password + `","recovery_code":"` + codes[0] + `"}`

	if rec := postLoginWith(t, handler, body); rec.Code != http.StatusOK {
		t.Fatalf("the first use: status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	rec := postLoginWith(t, handler, body)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("the SAME code worked twice: status = %d, want %d: %s",
			rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	if cookie := rec.Header().Get("Set-Cookie"); cookie != "" {
		t.Errorf("a spent code still issued a session: %s", cookie)
	}
}

// TestLogin_AUserWithNoPrivilegedMembershipNeedsNoSecondFactor is the
// anti-vacuity case for the whole file. Without it, a handler that refused
// EVERY login would pass every test above.
//
// An adopter has no membership at all: they browse the catalog and apply. There
// is nothing for a second factor to protect, and demanding one would make the
// public half of the product unusable.
func TestLogin_AUserWithNoPrivilegedMembershipNeedsNoSecondFactor(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := totpLoginHandler(t, env)

	const email, password = "adopter@refugio.test", "a-long-enough-password"
	seedAdopter(t, env, email, password)

	rec := postLoginWith(t, handler, `{"email":"`+email+`","password":"`+password+`"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d -- an adopter with no membership was asked for a second factor: %s",
			rec.Code, http.StatusOK, rec.Body.String())
	}

	claims := accessTokenFrom(t, rec)
	if len(claims.AMR) != 1 || claims.AMR[0] != "pwd" {
		t.Errorf("amr = %v, want [pwd] -- no second factor was used, so none may be claimed", claims.AMR)
	}
}

// TestLogin_APrivilegedUserWithoutTOTPEnrolledIsRefused closes the gap between
// "TOTP is mandatory for owners" and "this owner happens to have none".
//
// The reading that a not-yet-enrolled owner should be let through is the
// dangerous one: it makes the requirement opt-in by inaction, and every owner
// created before enrolment exists would be exempt forever. Refusing is the
// answer that keeps the rule a rule; the enrolment flow is what unblocks them.
func TestLogin_APrivilegedUserWithoutTOTPEnrolledIsRefused(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := totpLoginHandler(t, env)

	const email, password = "owner-noenrol@refugio.test", "a-long-enough-password"
	registerOwner(t, env, email, password, "owner-noenrol")
	// Deliberately NOT enrolled.

	rec := postLoginWith(t, handler, `{"email":"`+email+`","password":"`+password+`"}`)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d -- an owner with no second factor enrolled completed login: %s",
			rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

// TestLogin_AnAdminIsAsPrivilegedAsAnOwner covers the second half of §5.2's
// rule, which the mutation run found untested: every case above uses `owner`,
// so removing `admin` from the privileged set changed nothing.
//
// The spec names both roles, and an admin is not a lesser risk — they administer
// the shelter's animals, applications and members. A rule enforced for one of
// the two roles it names is a rule with a hole exactly where the second name is.
func TestLogin_AnAdminIsAsPrivilegedAsAnOwner(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := totpLoginHandler(t, env)

	const email, password = "admin-notenough@refugio.test", "a-long-enough-password"
	userID := seedAdopter(t, env, email, password)
	grantMembership(t, env, userID, "admin", "active")
	enrolTOTP(t, env, userID)

	rec := postLoginWith(t, handler, `{"email":"`+email+`","password":"`+password+`"}`)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d -- an admin completed login with a password alone: %s",
			rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	if cookie := rec.Header().Get("Set-Cookie"); cookie != "" {
		t.Errorf("a refused login set a session cookie: %s", cookie)
	}
}

// TestLogin_AVolunteerNeedsNoSecondFactor is what keeps the rule from being
// "every membership is privileged". A volunteer walks dogs; demanding an
// authenticator app of them would cost adoption more than it protects.
func TestLogin_AVolunteerNeedsNoSecondFactor(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := totpLoginHandler(t, env)

	const email, password = "volunteer@refugio.test", "a-long-enough-password"
	userID := seedAdopter(t, env, email, password)
	grantMembership(t, env, userID, "volunteer", "active")

	rec := postLoginWith(t, handler, `{"email":"`+email+`","password":"`+password+`"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d -- a volunteer was asked for a second factor: %s",
			rec.Code, http.StatusOK, rec.Body.String())
	}
}

// TestLogin_AnUnacceptedInvitationDoesNotDemandASecondFactor pins the `active`
// half of the privileged check, which the mutation run found unasserted.
//
// An `invited` membership grants nothing yet — the person has not accepted, and
// until they do they hold no role in that shelter. Counting it would lock a user
// out of their own account over a role somebody else offered them, which is a
// denial of service anyone can trigger by sending an invitation.
//
// A revoked membership is the same shape from the other end: the role is gone,
// and demanding its second factor is demanding one for nothing.
func TestLogin_AnUnacceptedInvitationDoesNotDemandASecondFactor(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := totpLoginHandler(t, env)

	for _, tt := range []struct {
		name   string
		status string
	}{
		{"an invitation not yet accepted", "invited"},
		{"a membership that was revoked", "revoked"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			email := "invitee-" + tt.status + "@refugio.test"
			const password = "a-long-enough-password"

			userID := seedAdopter(t, env, email, password)
			// An `owner` membership in the state under test. The role is the
			// privileged one on purpose: if status were ignored, this would
			// demand a second factor.
			grantMembership(t, env, userID, "owner", tt.status)

			rec := postLoginWith(t, handler, `{"email":"`+email+`","password":"`+password+`"}`)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d -- a %s membership demanded a second factor: %s",
					rec.Code, http.StatusOK, tt.status, rec.Body.String())
			}
		})
	}
}

// grantMembership gives the user a membership in a fresh shelter, with the role
// and status the caller asks for.
//
// It writes as the OWNER role because seeding a shelter outside registration is
// not something the tenant role does — and because the point of these tests is
// what LOGIN decides about a membership, not how the membership got there.
func grantMembership(t *testing.T, env *dbtest.Env, userID uuid.UUID, role, status string) {
	t.Helper()

	shelterID := uuid.New()
	dbtest.MustExec(t, env.OwnerPool,
		`INSERT INTO shelters (id, slug, display_name) VALUES ($1, $2, $3)`,
		shelterID, "seeded-"+shelterID.String()[:8], "Refugio Sembrado")
	dbtest.MustExec(t, env.OwnerPool,
		`INSERT INTO memberships (id, user_id, shelter_id, role, status) VALUES ($1, $2, $3, $4, $5)`,
		uuid.New(), userID, shelterID, role, status)
}

// TestLogin_AWrongTOTPCodeIsRefusedLikeAWrongPassword keeps the second factor
// from becoming its own oracle. If a bad code answered differently from a bad
// password, the refusal would tell a caller that the password was right.
func TestLogin_AWrongTOTPCodeIsRefusedLikeAWrongPassword(t *testing.T) {
	env := dbtest.Postgres(t)
	handler := totpLoginHandler(t, env)

	const email, password = "owner-badcode@refugio.test", "a-long-enough-password"
	userID := registerOwner(t, env, email, password, "owner-badcode")
	enrolTOTP(t, env, userID)

	badCode := postLoginWith(t, handler,
		`{"email":"`+email+`","password":"`+password+`","totp_code":"000000"}`)
	badPassword := postLoginWith(t, handler,
		`{"email":"`+email+`","password":"not-the-right-password","totp_code":"000000"}`)

	if badCode.Code != http.StatusUnauthorized {
		t.Fatalf("a wrong TOTP code: status = %d, want %d", badCode.Code, http.StatusUnauthorized)
	}
	if badCode.Body.String() != badPassword.Body.String() {
		t.Errorf("the refusals differ, which says which factor failed:\n  bad code:     %s\n  bad password: %s",
			badCode.Body.String(), badPassword.Body.String())
	}
	// And the content, for the same reason the login file learned: both bodies
	// go through one helper, so comparing them to each other cannot see a
	// message that names the failing factor on BOTH paths.
	body := strings.ToLower(badCode.Body.String())
	for _, leak := range []string{"totp", "code", "factor", "otp", "expired", "password"} {
		if strings.Contains(body, leak) {
			t.Errorf("the refusal names %q, which says which check failed: %s", leak, badCode.Body.String())
		}
	}
}
