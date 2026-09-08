package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/gentleman/mascotapp/apps/api/internal/auth"
	"github.com/gentleman/mascotapp/apps/api/internal/db"
	"github.com/gentleman/mascotapp/apps/api/internal/db/sqlcgen"
)

// registerRequest is the registration form.
//
// There is no `status` field, and its absence is the whole point of the spec's
// requirement: a shelter lands `pending_verification` regardless of what the
// caller submits (LT-2). Dropping it at the edge is the friendly half — the
// binding half is that `app_tenant` holds no INSERT privilege on
// `shelters.status`, so even a statement that named it would be refused.
type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	FullName    string `json:"full_name"`
	Phone       string `json:"phone"`
	ShelterName string `json:"shelter_name"`
	ShelterSlug string `json:"shelter_slug"`
	LegalName   string `json:"legal_name"`
	Country     string `json:"country"`
	State       string `json:"state"`
	City        string `json:"city"`
}

// registerResponse names what was created. It carries no token: registration
// and session issuance are separate steps, so a shelter that registers still
// has to log in, and a failure to issue a session cannot look like a failure to
// register.
type registerResponse struct {
	UserID    string `json:"user_id"`
	ShelterID string `json:"shelter_id"`
}

// minPasswordBytes is a floor, not a policy. Real password rules belong with
// the credential rotation work; this only keeps an empty or one-character
// password from reaching Argon2id and being hashed perfectly well.
const minPasswordBytes = 12

// RegisterHandler creates a shelter and its founding owner.
//
// It takes TWO pools because P2-D6 makes registration two transactions across
// two roles, and they cannot be collapsed into one:
//
//  1. WithAuthUser inserts the `users` row as `app_auth`, idempotent on the
//     citext UNIQUE email.
//  2. WithTenant inserts `shelters` AND the founder's `owner` membership as
//     `app_tenant`, atomically.
//
// The order is the design, not an implementation detail: the membership
// references the user, so the reverse is impossible. If (2) fails, a user exists
// with no shelter — harmless, and re-running registration resolves it because
// (1) is idempotent. That is the entire compensation, and it is why (1) carries
// ON CONFLICT DO NOTHING rather than why it happens to.
func RegisterHandler(authPool db.Beginner, tenantPool db.Beginner) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req registerRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			badRequest(w, "the request body is not valid JSON")
			return
		}

		if field, ok := req.missingField(); !ok {
			badRequest(w, field+" is required")
			return
		}
		if len(req.Password) < minPasswordBytes {
			badRequest(w, "password must be at least 12 characters")
			return
		}

		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			// Nothing has been written yet, so there is nothing to undo.
			serverError(w, r, "hashing the password", err)
			return
		}

		ctx := r.Context()
		userID := uuid.New()
		shelterID := uuid.New()

		// (1) The user. On conflict this writes nothing, so the id generated
		// above may not be the one that ends up in the row — which is why the
		// existing id is read back below rather than assumed.
		err = db.WithAuthUser(ctx, authPool, userID, func(ctx context.Context, tx pgx.Tx) error {
			return sqlcgen.New(tx).InsertUser(ctx, sqlcgen.InsertUserParams{
				ID:           userID,
				Email:        req.Email,
				PasswordHash: pgtype.Text{String: hash, Valid: true},
				FullName:     optionalText(req.FullName),
				Phone:        optionalText(req.Phone),
			})
		})
		if err != nil {
			serverError(w, r, "creating the user", err)
			return
		}

		// Re-read rather than trust the generated id. A conflicting insert
		// writes no row, so on the second registration of one address `userID`
		// names a user that does not exist, and the membership below would fail
		// its foreign key. Asking the database which id that address actually
		// has is the only answer that holds in both cases.
		existingID, err := lookupUserID(ctx, authPool, req.Email)
		if err != nil {
			serverError(w, r, "reading the registered user back", err)
			return
		}
		userID = existingID

		// (2) The shelter and the founder's membership, atomically. Anything
		// that fails here leaves the user from (1) in place: that is the
		// compensation, and it is deliberate.
		err = db.WithTenant(ctx, tenantPool, shelterID, func(ctx context.Context, tx pgx.Tx) error {
			queries := sqlcgen.New(tx)

			if err := queries.CreateShelter(ctx, sqlcgen.CreateShelterParams{
				ID:           shelterID,
				Slug:         req.ShelterSlug,
				DisplayName:  req.ShelterName,
				LegalName:    optionalText(req.LegalName),
				Country:      optionalText(req.Country),
				State:        optionalText(req.State),
				City:         optionalText(req.City),
				ContactEmail: optionalText(req.Email),
			}); err != nil {
				return err
			}

			// The founder is not invited, they are already here, so this is not
			// InviteMember: that query hard-codes status 'invited', which would
			// leave the new shelter with nobody able to administer it.
			//
			// `accepted_at` is left NULL, and that is the schema talking rather
			// than an omission. 00015 grants `app_tenant` INSERT on
			// (id, user_id, shelter_id, role, invited_by, status) and NOT on
			// `accepted_at`, which it grants on UPDATE instead — the column
			// belongs to the invite-then-accept flow. A founder accepted no
			// invitation because none was sent, so NULL is the honest value;
			// writing now() would need the grant widened, and widening a grant
			// to fit a handler is how a boundary erodes.
			_, err := tx.Exec(ctx,
				`INSERT INTO memberships (id, user_id, shelter_id, role, status)
				 VALUES ($1, $2, $3, 'owner', 'active')`,
				uuid.New(), userID, shelterID,
			)
			return err
		})
		if err != nil {
			serverError(w, r, "creating the shelter", err)
			return
		}

		writeJSON(w, http.StatusCreated, registerResponse{
			UserID:    userID.String(),
			ShelterID: shelterID.String(),
		})
	})
}

// missingField returns the name of the first required field that is empty. It
// reports the field rather than a generic "invalid request" because a 400 that
// does not say what is wrong is a 400 the caller cannot act on.
func (r registerRequest) missingField() (string, bool) {
	for _, field := range []struct {
		name  string
		value string
	}{
		{"email", r.Email},
		{"password", r.Password},
		{"shelter_name", r.ShelterName},
		{"shelter_slug", r.ShelterSlug},
	} {
		if strings.TrimSpace(field.value) == "" {
			return field.name, false
		}
	}
	return "", true
}

// lookupUserID answers which user id an address actually holds, which is not
// always the id the caller just generated — see the call site.
func lookupUserID(ctx context.Context, authPool db.Beginner, email string) (uuid.UUID, error) {
	lookup, ok := authPool.(db.TxBeginner)
	if !ok {
		return uuid.Nil, errors.New("httpapi: the auth pool cannot open a read-only transaction")
	}

	var id uuid.UUID
	err := db.WithAuthLookup(ctx, lookup, func(ctx context.Context, tx pgx.Tx) error {
		row, err := sqlcgen.New(tx).GetUserCredentialsByEmail(ctx, email)
		if err != nil {
			return err
		}
		id = row.ID
		return nil
	})
	return id, err
}

// optionalText maps an empty string to SQL NULL. An empty full_name is an
// absent one, and storing "" would make "never supplied" indistinguishable from
// "deliberately blank" for every reader afterwards.
func optionalText(value string) pgtype.Text {
	if strings.TrimSpace(value) == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func badRequest(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": message})
}

// serverError logs the cause and tells the caller nothing about it. The message
// that reaches the client is fixed: a database error text can name a column, a
// constraint, or the fact that an email is already registered, and registration
// is exactly the endpoint where that last one is an enumeration oracle.
func serverError(w http.ResponseWriter, r *http.Request, doing string, err error) {
	slog.ErrorContext(r.Context(), "registration failed", "doing", doing, "error", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "registration failed"})
}
