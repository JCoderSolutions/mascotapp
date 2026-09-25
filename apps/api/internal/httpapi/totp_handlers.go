package httpapi

import (
	"context"
	"encoding/base32"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/gentleman/mascotapp/apps/api/internal/auth"
	"github.com/gentleman/mascotapp/apps/api/internal/db"
)

// TOTP enrolment (identity-and-session / *TOTP is mandatory for owner and
// admin roles* -- these two handlers are the precondition every scenario in
// that requirement assumes: an owner or admin cannot have "TOTP enrolled"
// unless something enrolled it).
//
// Both routes sit behind RequireAuth and nothing else -- an unscoped route
// like choosing a shelter or reading your own profile, per middleware_auth.go's
// own comment. There is no tenant here: a second factor belongs to the user,
// not to any one shelter they belong to.

// totpIssuerName is the `issuer` parameter written into every otpauth:// URI,
// which is what an authenticator app shows next to the account label.
const totpIssuerName = "mascotapp"

// totpSecretEncoding is unpadded base32, matching both the `secret` query
// parameter TOTPURI already writes and what an authenticator app's manual
// entry field expects.
var totpSecretEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// totpEnrolResponse is what TOTPEnrolHandler returns.
type totpEnrolResponse struct {
	// Status is "pending" until Verify confirms the caller actually holds a
	// working authenticator for this secret.
	Status string `json:"status"`

	// Secret is the same bytes OTPAuthURI's `secret` parameter carries, base32
	// for an app that wants manual entry instead of a QR scan.
	Secret string `json:"secret"`

	OTPAuthURI string `json:"otpauth_uri"`

	// RecoveryCodes is shown exactly once. Nothing after this response can
	// recover them -- only their SHA-256 hash is stored (recovery.go) -- and a
	// second call to this handler REPLACES the set rather than re-serving it.
	RecoveryCodes []string `json:"recovery_codes"`
}

// TOTPEnrolHandler starts, or restarts, TOTP enrolment for the authenticated
// caller.
//
// It persists immediately rather than waiting for Verify, and a repeated call
// is deliberately SAFE rather than merely idempotent: RegenerateRecoveryCodes'
// delete-then-insert means the previous attempt's codes stop being redeemable
// the moment a new set lands, so a caller who lost the first response before
// scanning it can retry without ever leaving two live sets of plaintext
// credentials for the same account, and the lost response can never be
// reconstructed into a working one afterwards.
func TOTPEnrolHandler(authPool db.Beginner, envelope *auth.Envelope) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := ClaimsFromContext(r.Context())
		if !ok {
			// Mounted without RequireAuth above it -- a wiring bug, not
			// something a caller did.
			http.Error(w, "server misconfigured", http.StatusInternalServerError)
			return
		}

		secret, err := auth.NewTOTPSecret()
		if err != nil {
			serverError(w, r, "generating a TOTP secret", err)
			return
		}
		codes, err := auth.NewRecoveryCodes()
		if err != nil {
			serverError(w, r, "generating recovery codes", err)
			return
		}
		sealed, err := envelope.Seal(secret, claims.Subject)
		if err != nil {
			serverError(w, r, "sealing the TOTP secret", err)
			return
		}

		ctx := r.Context()
		var email string
		err = db.WithAuthUser(ctx, authPool, claims.Subject, func(ctx context.Context, tx pgx.Tx) error {
			if err := tx.QueryRow(ctx,
				`SELECT email FROM users WHERE id = $1`, claims.Subject,
			).Scan(&email); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx,
				`UPDATE users SET totp_secret_enc = $1, updated_at = now() WHERE id = $2`,
				sealed, claims.Subject,
			); err != nil {
				return err
			}
			return auth.RegenerateRecoveryCodes(ctx, tx, claims.Subject, codes)
		})
		if err != nil {
			serverError(w, r, "persisting the TOTP enrolment", err)
			return
		}

		uri, err := auth.TOTPURI(secret, totpIssuerName, email)
		if err != nil {
			serverError(w, r, "building the TOTP URI", err)
			return
		}

		plaintexts := make([]string, 0, len(codes))
		for _, code := range codes {
			plaintexts = append(plaintexts, code.Plaintext)
		}

		writeJSON(w, http.StatusOK, totpEnrolResponse{
			Status:        "pending",
			Secret:        totpSecretEncoding.EncodeToString(secret),
			OTPAuthURI:    uri,
			RecoveryCodes: plaintexts,
		})
	})
}

type totpVerifyRequest struct {
	Code string `json:"code"`
}

type totpVerifyResponse struct {
	Status string `json:"status"`
}

// TOTPVerifyHandler confirms the caller holds a working authenticator for the
// secret TOTPEnrolHandler most recently issued, and answers "enrolled" once
// it does.
//
// now is injected for the same reason LoginHandler's is: a code's 30-second
// step has to be testable without sleeping through one.
func TOTPVerifyHandler(authPool db.Beginner, envelope *auth.Envelope, now func() time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := ClaimsFromContext(r.Context())
		if !ok {
			http.Error(w, "server misconfigured", http.StatusInternalServerError)
			return
		}

		var req totpVerifyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			badRequest(w, "the request body is not valid JSON")
			return
		}

		ctx := r.Context()
		var sealed []byte
		err := db.WithAuthUser(ctx, authPool, claims.Subject, func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				`SELECT totp_secret_enc FROM users WHERE id = $1`, claims.Subject,
			).Scan(&sealed)
		})
		if err != nil {
			serverError(w, r, "reading the pending TOTP secret", err)
			return
		}
		if len(sealed) == 0 {
			badRequest(w, "no TOTP enrolment is pending")
			return
		}

		secret, err := envelope.Open(sealed, claims.Subject)
		if err != nil {
			// A blob that will not open is the same refusal as a wrong code:
			// there is nothing a caller can act on by learning which.
			unauthorizedTOTP(w)
			return
		}
		if !auth.VerifyTOTP(secret, req.Code, now()) {
			unauthorizedTOTP(w)
			return
		}

		writeJSON(w, http.StatusOK, totpVerifyResponse{Status: "enrolled"})
	})
}

// unauthorizedTOTP is TOTPVerifyHandler's one refusal for a code that failed
// to validate. Kept separate from unauthorizedLogin: this endpoint verifies
// no password, so its failures are not the login existence oracle that
// function exists to close, and reusing its body would be borrowing a
// guarantee this handler does not need.
func unauthorizedTOTP(w http.ResponseWriter) {
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid code"})
}
