package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/gentleman/mascotapp/apps/api/internal/auth"
	"github.com/gentleman/mascotapp/apps/api/internal/db"
	"github.com/gentleman/mascotapp/apps/api/internal/db/sqlcgen"
)

// The refresh cookie, spelled out because every attribute carries a property
// (P2-D9).
const (
	// refreshCookieName keeps the __Secure- prefix, which binds the browser to
	// two rules: the cookie may only be set over HTTPS, and a non-secure origin
	// cannot overwrite it. The __Host- prefix is stronger and was rejected —
	// it requires Path=/, and a refresh cookie riding every request is a wider
	// blast radius than the prefix is worth.
	refreshCookieName = "__Secure-mascotapp_refresh"

	// refreshCookiePath scopes the cookie to the routes that spend it. The
	// browser attaches it nowhere else, so an XSS on a catalog page cannot ride
	// it even if it finds a way to make a request.
	refreshCookiePath = "/api/v1/auth"
)

// RoleUnscoped is the `role` claim of a token that has not chosen a tenant.
//
// `AccessClaims.validate` refuses an empty role, and it is right to: RBAC reads
// that claim, and a token whose role is "" would evaluate as "no permissions"
// in one reading and "not set, fall through" in another. But a role is a
// property of a MEMBERSHIP, and a membership belongs to one shelter — so a
// token with no `shelter_id` cannot honestly carry one either.
//
// Naming the state is the way out. This value is not a role with no
// permissions; it is the assertion that no tenant-scoped role has been claimed
// yet, and `POST /auth/session/shelter` is what replaces it after verifying an
// active membership. Any RBAC check that matches it is a bug in that check: it
// means a tenant-scoped decision was made on a token that named no tenant.
const RoleUnscoped = "unscoped"

// dummyHashForTiming is a real Argon2id encoding of a password nobody has.
//
// It exists so an unknown email costs the same as a known one. Without it,
// login returns immediately when the address is not found and spends ~100ms
// hashing when it is: an unauthenticated caller with a stopwatch enumerates the
// user table without ever reading a response body. The body being identical
// (which it is) does not close that channel — the clock does.
const dummyHashForTiming = "$argon2id$v=19$m=65536,t=3,p=1$" +
	"c29tZS1zYWx0LXZhbHVl$" +
	"ZG8gbm90IG1hdGNoIGFueXRoaW5nIGF0IGFsbCB3aGF0ZXY"

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// LoginHandler verifies a password and issues a session.
//
// `now` is injected rather than read from the clock so token expiry is testable
// without sleeping. It is the same seam `RequireAuth` uses, for the same reason.
//
// The issued token carries NO shelter_id. A founder belongs to exactly one
// shelter, which makes putting it straight in the token the tempting shortcut,
// and that shortcut is what the exchange step exists to prevent: this claim is
// the sole input to tenant resolution (ADR-0008), and `uuid.Nil` would pass a
// presence check and land in `WithTenant`. Login asserts WHO you are; which
// tenant you act as is a separate assertion after an explicit choice.
func LoginHandler(authPool db.Beginner, issuer *auth.TokenIssuer, now func() time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			badRequest(w, "the request body is not valid JSON")
			return
		}
		// Refused before Argon2id sees it. Hashing an empty password costs
		// 64 MiB and three passes on a 512 MiB instance, and an
		// unauthenticated caller should not be able to spend that with `{}`.
		if strings.TrimSpace(req.Email) == "" || req.Password == "" {
			badRequest(w, "email and password are required")
			return
		}

		ctx := r.Context()

		credentials, found, err := findCredentials(ctx, authPool, req.Email)
		if err != nil {
			serverError(w, r, "looking up the credential", err)
			return
		}

		// The hash to verify against, and the comparison itself, happen whether
		// or not the account exists. Returning early on `!found` is what turns
		// this endpoint into a timing oracle.
		stored := dummyHashForTiming
		if found && credentials.PasswordHash.Valid {
			stored = credentials.PasswordHash.String
		}

		matches, err := auth.VerifyPassword(stored, req.Password)
		if err != nil {
			// A hash that will not parse is a corrupt row, not a wrong
			// password (see auth.ErrInvalidHash). It is logged as the incident
			// it is, and the caller still gets the ordinary refusal: telling
			// them their stored credential is broken tells them the account
			// exists.
			if !errors.Is(err, auth.ErrInvalidHash) {
				serverError(w, r, "verifying the password", err)
				return
			}
			slog.ErrorContext(ctx, "a stored password hash could not be parsed", "user_id", credentials.ID)
			matches = false
		}

		if !found || !matches || credentials.Status != "active" {
			unauthorizedLogin(w)
			return
		}

		// Every login starts its own family, so revoking one device's session
		// leaves the others alone.
		var cookieValue string
		err = db.WithAuthUser(ctx, authPool, credentials.ID, func(ctx context.Context, tx pgx.Tx) error {
			var issueErr error
			cookieValue, issueErr = auth.IssueRefreshToken(ctx, tx, credentials.ID, now())
			return issueErr
		})
		if err != nil {
			serverError(w, r, "issuing the refresh token", err)
			return
		}

		accessToken, err := issuer.Issue(auth.AccessClaims{
			Subject: credentials.ID,
			Role:    RoleUnscoped,
			AMR:     []string{"pwd"},
		}, now())
		if err != nil {
			// The refresh token is already committed. That is survivable: it is
			// a session the client never receives and that expires on its own.
			// The reverse order — cookie set, token minted, refresh write fails —
			// would hand out a credential with no row behind it.
			serverError(w, r, "issuing the access token", err)
			return
		}

		//nolint:gosec // G124 false positive: Secure and HttpOnly are both set,
		// and SameSite is None BY DECISION (P2-D9), not by omission. The origins
		// are separate, so Lax or Strict would never send this cookie at all --
		// the rule gosec applies assumes a same-origin deployment this project
		// explicitly does not have. Every attribute is asserted one at a time by
		// TestLogin_SetsTheRefreshCookieWithEveryAttributeP2D9Requires, and
		// config.Load refuses to boot if the origins ever converge.
		http.SetCookie(w, &http.Cookie{
			Name:     refreshCookieName,
			Value:    cookieValue,
			Path:     refreshCookiePath,
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteNoneMode,
		})

		writeJSON(w, http.StatusOK, loginResponse{
			AccessToken: accessToken,
			TokenType:   "Bearer",
			ExpiresIn:   int(auth.AccessTokenLifetime.Seconds()),
		})
	})
}

// findCredentials reports whether the address has an account, separately from
// whether the lookup failed.
//
// "No such user" is not an error: it is one of two ordinary outcomes, and
// collapsing it into `err` would make the caller unable to tell a missing
// account from a database that is down — which are a 401 and a 500.
func findCredentials(ctx context.Context, authPool db.Beginner, email string) (sqlcgen.GetUserCredentialsByEmailRow, bool, error) {
	lookup, ok := authPool.(db.TxBeginner)
	if !ok {
		return sqlcgen.GetUserCredentialsByEmailRow{}, false, errors.New("httpapi: the auth pool cannot open a read-only transaction")
	}

	var row sqlcgen.GetUserCredentialsByEmailRow
	err := db.WithAuthLookup(ctx, lookup, func(ctx context.Context, tx pgx.Tx) error {
		var lookupErr error
		row, lookupErr = sqlcgen.New(tx).GetUserCredentialsByEmail(ctx, email)
		return lookupErr
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return sqlcgen.GetUserCredentialsByEmailRow{}, false, nil
	}
	if err != nil {
		return sqlcgen.GetUserCredentialsByEmailRow{}, false, err
	}
	return row, true, nil
}

// unauthorizedLogin is ONE refusal for every reason a login can fail: no such
// address, wrong password, unparseable hash, suspended account.
//
// They are indistinguishable on purpose. Any difference — the status, a word in
// the body, a header present on one path and not the other — lets an
// unauthenticated caller ask whether an address is registered and get an answer.
// This is the same defect as returning a driver's error text, one layer up:
// there the leak was the database's helpfulness, here it would be ours.
func unauthorizedLogin(w http.ResponseWriter) {
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
}
