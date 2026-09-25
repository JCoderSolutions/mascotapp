package httpapi_test

import (
	"context"
	"encoding/base32"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gentleman/mascotapp/apps/api/internal/auth"
	"github.com/gentleman/mascotapp/apps/api/internal/db"
	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
	"github.com/gentleman/mascotapp/apps/api/internal/httpapi"
)

// TOTP enrolment (identity-and-session / *TOTP is mandatory for owner and
// admin roles* -- enrolment is the precondition every login-time scenario in
// login_totp_test.go assumes) and the design's Open Question: "the API
// returns them [recovery codes] exactly once and never again".
//
// Every test here goes through the real handlers against a real database,
// wired exactly as the design lists: totp.go + envelope.go + recovery.go,
// reached through RequireAuth like any other authenticated-but-unscoped
// route (choosing a shelter, reading your own profile).

var totpSecretEncodingForTest = base32.StdEncoding.WithPadding(base32.NoPadding)

func totpEnrolFixture(t *testing.T, env *dbtest.Env) http.Handler {
	t.Helper()

	envelope, err := auth.NewEnvelope(testKEK)
	if err != nil {
		t.Fatalf("building the envelope: %v", err)
	}
	return httpapi.TOTPEnrolHandler(env.AuthPool, envelope)
}

func totpVerifyFixture(t *testing.T, env *dbtest.Env, now time.Time) http.Handler {
	t.Helper()

	envelope, err := auth.NewEnvelope(testKEK)
	if err != nil {
		t.Fatalf("building the envelope: %v", err)
	}
	return httpapi.TOTPVerifyHandler(env.AuthPool, envelope, func() time.Time { return now })
}

// authedTOTPRequest runs one request through RequireAuth for userID, exactly
// as a real client hitting a mounted route would -- never by reaching into
// the context directly, which would assert a shortcut nothing in production
// takes.
func authedTOTPRequest(t *testing.T, handler http.Handler, path, body string, userID uuid.UUID, now time.Time) *httptest.ResponseRecorder {
	t.Helper()

	token, err := newTestIssuer(t).Issue(auth.AccessClaims{
		Subject: userID,
		Role:    httpapi.RoleUnscoped,
		AMR:     []string{"pwd"},
	}, now)
	if err != nil {
		t.Fatalf("issuing a test token: %v", err)
	}

	router := chi.NewRouter()
	router.With(httpapi.RequireAuth(newTestIssuer(t), func() time.Time { return now })).
		Post(path, handler.ServeHTTP)

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

type totpEnrolTestResponse struct {
	Status        string   `json:"status"`
	Secret        string   `json:"secret"`
	OTPAuthURI    string   `json:"otpauth_uri"`
	RecoveryCodes []string `json:"recovery_codes"`
}

func decodeEnrolResponse(t *testing.T, rec *httptest.ResponseRecorder) totpEnrolTestResponse {
	t.Helper()

	var got totpEnrolTestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding the enrol response: %v: %s", err, rec.Body.String())
	}
	return got
}

// TestTOTPEnrol_ReturnsASecretURIAndTenUniqueRecoveryCodes is the RED case for
// the first half of the task: enrol generates all three, and the codes are
// pairwise distinct -- ten copies of one code would technically satisfy "ten
// codes" while protecting nothing.
func TestTOTPEnrol_ReturnsASecretURIAndTenUniqueRecoveryCodes(t *testing.T) {
	env := dbtest.Postgres(t)
	now := time.Now()
	userID := registerOwner(t, env, "totp-enrol@refugio.test", "a-long-enough-password", "totp-enrol")

	rec := authedTOTPRequest(t, totpEnrolFixture(t, env), "/api/v1/auth/totp/enrol", "{}", userID, now)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	got := decodeEnrolResponse(t, rec)
	if got.Status != "pending" {
		t.Errorf("status = %q, want %q -- verify has not confirmed anything yet", got.Status, "pending")
	}
	if !strings.HasPrefix(got.OTPAuthURI, "otpauth://totp/") {
		t.Errorf("otpauth_uri = %q, want an otpauth://totp/ URI", got.OTPAuthURI)
	}

	secret, err := totpSecretEncodingForTest.DecodeString(got.Secret)
	if err != nil {
		t.Fatalf("secret %q does not decode as unpadded base32: %v", got.Secret, err)
	}
	if len(secret) != auth.TOTPSecretLength {
		t.Errorf("decoded secret is %d bytes, want %d", len(secret), auth.TOTPSecretLength)
	}

	if len(got.RecoveryCodes) != auth.RecoveryCodeCount {
		t.Fatalf("got %d recovery codes, want %d", len(got.RecoveryCodes), auth.RecoveryCodeCount)
	}
	seen := make(map[string]bool, len(got.RecoveryCodes))
	for _, code := range got.RecoveryCodes {
		if seen[code] {
			t.Errorf("recovery code %q was issued twice in the same set", code)
		}
		seen[code] = true
	}
}

// TestTOTPEnrol_ASecondEnrolInvalidatesTheFirstRecoveryCodes is the task's
// named RED case: "a second read of the same enrolment never returns the
// plaintext codes again". A second call to this handler is that second
// read -- it REPLACES the pending attempt rather than re-serving it, and the
// proof is not merely that the response differs: the FIRST set must stop
// being a redeemable credential, or an attacker holding a leaked first
// response could still spend it after the caller retried.
func TestTOTPEnrol_ASecondEnrolInvalidatesTheFirstRecoveryCodes(t *testing.T) {
	env := dbtest.Postgres(t)
	now := time.Now()
	userID := registerOwner(t, env, "totp-second-read@refugio.test", "a-long-enough-password", "totp-second-read")
	handler := totpEnrolFixture(t, env)

	first := decodeEnrolResponse(t, authedTOTPRequest(t, handler, "/api/v1/auth/totp/enrol", "{}", userID, now))
	second := decodeEnrolResponse(t, authedTOTPRequest(t, handler, "/api/v1/auth/totp/enrol", "{}", userID, now))

	firstCodes := make(map[string]bool, len(first.RecoveryCodes))
	for _, code := range first.RecoveryCodes {
		firstCodes[code] = true
	}
	for _, code := range second.RecoveryCodes {
		if firstCodes[code] {
			t.Errorf("the second enrol response repeated a code from the first: %q", code)
		}
	}

	err := db.WithAuthUser(context.Background(), env.AuthPool, userID, func(ctx context.Context, tx pgx.Tx) error {
		return auth.RedeemRecoveryCode(ctx, tx, first.RecoveryCodes[0])
	})
	if !errors.Is(err, auth.ErrRecoveryCodeInvalid) {
		t.Errorf("redeeming a code from the FIRST enrolment after a second one succeeded: err=%v, want %v",
			err, auth.ErrRecoveryCodeInvalid)
	}
}

// TestTOTPVerify_AValidCodeActivatesTheEnrolment is the task's second named
// RED case: verify confirms a submitted code and moves the enrolment from
// pending to enrolled.
func TestTOTPVerify_AValidCodeActivatesTheEnrolment(t *testing.T) {
	env := dbtest.Postgres(t)
	now := time.Now()
	userID := registerOwner(t, env, "totp-verify-ok@refugio.test", "a-long-enough-password", "totp-verify-ok")

	enrolled := decodeEnrolResponse(t,
		authedTOTPRequest(t, totpEnrolFixture(t, env), "/api/v1/auth/totp/enrol", "{}", userID, now))
	secret, err := totpSecretEncodingForTest.DecodeString(enrolled.Secret)
	if err != nil {
		t.Fatalf("decoding the enrolled secret: %v", err)
	}

	body := `{"code":"` + currentTOTP(t, secret) + `"}`
	rec := authedTOTPRequest(t, totpVerifyFixture(t, env, now), "/api/v1/auth/totp/verify", body, userID, now)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding the verify response: %v", err)
	}
	if got.Status != "enrolled" {
		t.Errorf("status = %q, want %q -- a valid code did not activate the enrolment", got.Status, "enrolled")
	}
}

// TestTOTPVerify_AnInvalidCodeIsRefused is the failure mode the confirmation
// exists to catch: a wrong code moves nothing from pending to enrolled.
func TestTOTPVerify_AnInvalidCodeIsRefused(t *testing.T) {
	env := dbtest.Postgres(t)
	now := time.Now()
	userID := registerOwner(t, env, "totp-verify-bad@refugio.test", "a-long-enough-password", "totp-verify-bad")

	authedTOTPRequest(t, totpEnrolFixture(t, env), "/api/v1/auth/totp/enrol", "{}", userID, now)

	rec := authedTOTPRequest(t, totpVerifyFixture(t, env, now), "/api/v1/auth/totp/verify", `{"code":"000000"}`, userID, now)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

// TestTOTPVerify_WithNoPendingEnrolmentIsRefused is the anti-vacuity case:
// without it, a handler that always answered 401 would pass the test above.
func TestTOTPVerify_WithNoPendingEnrolmentIsRefused(t *testing.T) {
	env := dbtest.Postgres(t)
	now := time.Now()
	userID := seedAdopter(t, env, "totp-verify-none@refugio.test", "a-long-enough-password")

	rec := authedTOTPRequest(t, totpVerifyFixture(t, env, now), "/api/v1/auth/totp/verify", `{"code":"123456"}`, userID, now)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d -- nothing was enrolled to verify: %s",
			rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}
