package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/gentleman/mascotapp/apps/api/internal/auth"
	"github.com/gentleman/mascotapp/apps/api/internal/db"
	"github.com/gentleman/mascotapp/apps/api/internal/db/sqlcgen"
)

// RefreshHandler spends the refresh cookie and returns a rotated one with a
// fresh access token.
//
// The origin check wraps EVERYTHING. This route and logout are the only two the
// refresh cookie is sent to, and it is sent with SameSite=None, so they are the
// two routes a cross-site page can trigger with the session attached. Nothing —
// not the cookie parse, not a transaction — runs for a request that fails it.
//
// The issued token's `amr` is exactly `[pwd]`, by the user's decision of
// 2026-09-14. `refresh_tokens` does not record how a session was authenticated,
// so a refresh cannot know whether a second factor was used at login.
// Preserving `otp` would need that stored, and it would make a stolen refresh
// cookie carry the second factor for the life of its family. With `[pwd]`, a
// refresh never asserts a factor it did not itself verify, and any route that
// demands `otp` sends the holder back through login.
func RefreshHandler(authPool db.Beginner, issuer *auth.TokenIssuer, allowlist *OriginAllowlist, now func() time.Time) http.Handler {
	return RequireTrustedOrigin(allowlist)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(refreshCookieName)
		if err != nil {
			refuseRefresh(w)
			return
		}
		presented, err := auth.ParseRefreshCookie(cookie.Value)
		if err != nil {
			refuseRefresh(w)
			return
		}

		ctx := r.Context()

		// The callback returns ONLY the infrastructure error. A refusal travels
		// in the outcome and the transaction COMMITS — for reuse, that commit
		// is the family revocation, and returning the refusal as an error
		// would roll the containment back and leave the thief a live token.
		var outcome auth.RotationOutcome
		err = db.WithAuthUser(ctx, authPool, presented.UserID, func(ctx context.Context, tx pgx.Tx) error {
			var rotateErr error
			outcome, rotateErr = auth.RotateRefreshToken(ctx, tx, presented.Secret, now())
			return rotateErr
		})
		if err != nil {
			// Two errors here are refusals, not failures. Losing a concurrent
			// rotation comes back wrapped in ErrRefreshTokenInvalid, and a cookie
			// whose user prefix decodes to the zero uuid makes WithAuthUser
			// refuse to open a scope. Answering either with a 500 would give the
			// caller a response that no other refusal gives.
			if errors.Is(err, auth.ErrRefreshTokenInvalid) || errors.Is(err, db.ErrNoAuthUser) {
				refuseRefresh(w)
				return
			}
			serverError(w, r, "rotating the refresh token", err)
			return
		}
		if outcome.Refusal != nil {
			// The one refusal the SERVER must be able to see. The caller gets
			// the same reply as every other refusal; the alarm goes here.
			if errors.Is(outcome.Refusal, auth.ErrRefreshTokenReused) {
				slog.WarnContext(ctx, "a spent refresh token was presented and its family has been revoked",
					"user_id", presented.UserID)
			}
			refuseRefresh(w)
			return
		}

		// The subject is the cookie's user prefix, and that is safe only because
		// of where the rotation ran: inside WithAuthUser(presented.UserID), whose
		// policy hides every row belonging to anyone else. A successful rotation
		// means the hash matched a row of THAT user. A lying prefix scopes the
		// lookup to rows that do not contain the hash and never reaches here.
		accessToken, err := issuer.Issue(auth.AccessClaims{
			Subject: presented.UserID,
			Role:    RoleUnscoped,
			AMR:     []string{"pwd"},
		}, now())
		if err != nil {
			// The rotation is committed and its cookie is never delivered. That
			// session is unreachable, and the holder's previous cookie is now
			// spent — so their next attempt reads as reuse and revokes the
			// family. Harsh, and correct: failing closed on the path that mints
			// credentials.
			serverError(w, r, "issuing the access token", err)
			return
		}

		setRefreshCookie(w, outcome.Cookie)
		writeJSON(w, http.StatusOK, loginResponse{
			AccessToken: accessToken,
			TokenType:   "Bearer",
			ExpiresIn:   int(auth.AccessTokenLifetime.Seconds()),
		})
	}))
}

// LogoutHandler ends the session the presented cookie belongs to.
//
// It answers 204 whether or not there was anything to end: no cookie, a
// malformed one, or a session already over. Logging out twice is not an error,
// and a logout that answered differently for a live session would be an oracle
// over which cookies are valid.
//
// The one exception is an infrastructure failure, which answers 500 WITHOUT
// clearing the cookie. Telling a browser "logged out" while the session is still
// live — and taking away the only handle it had to retry — is worse than an
// honest error.
func LogoutHandler(authPool db.Beginner, allowlist *OriginAllowlist) http.Handler {
	return RequireTrustedOrigin(allowlist)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(refreshCookieName); err == nil {
			if presented, err := auth.ParseRefreshCookie(cookie.Value); err == nil {
				if err := endSession(r.Context(), authPool, presented); err != nil {
					serverError(w, r, "ending the session", err)
					return
				}
			}
		}

		clearRefreshCookie(w)
		w.WriteHeader(http.StatusNoContent)
	}))
}

// endSession revokes the whole family the presented cookie belongs to.
//
// The FAMILY, not the presented row. Rotation leaves one live row per family and
// it is rarely the one an older cookie points at, so a single-row revoke does
// nothing for a stale cookie while the successor that opens the session lives
// on. The same gap opens with no old cookie at all when a refresh is in flight
// during logout.
//
// Under the family lock, because Judgment Day (T-02-021) showed why a bare
// family revoke is not enough: it fixes its rows at its own snapshot, and a
// successor inserted by a concurrent rotation afterwards survives it. The lock
// serialises this with any rotation of the same family.
//
// A cookie whose secret matches nothing is not an error. It is a session that is
// already over, or one that never existed, and both end the same way.
func endSession(ctx context.Context, authPool db.Beginner, presented auth.RefreshCookie) error {
	err := db.WithAuthUser(ctx, authPool, presented.UserID, func(ctx context.Context, tx pgx.Tx) error {
		queries := sqlcgen.New(tx)

		row, err := queries.GetRefreshTokenByHash(ctx, auth.HashRefreshSecret(presented.Secret))
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}

		if err := queries.LockRefreshTokenFamily(ctx, row.FamilyID.String()); err != nil {
			return err
		}
		_, err = queries.RevokeRefreshTokenFamily(ctx, row.FamilyID)
		return err
	})
	// A zero-uuid prefix cannot open a scope, and there is nothing it could
	// have revoked.
	if errors.Is(err, db.ErrNoAuthUser) {
		return nil
	}
	return err
}

// refuseRefresh is ONE reply for every reason a refresh fails.
//
// It also tells the browser to drop the cookie. Every refusal means the cookie
// will never work again — spent, revoked, expired, or never ours — and a client
// that keeps presenting a dead credential reads each 401 as a new problem.
func refuseRefresh(w http.ResponseWriter) {
	clearRefreshCookie(w)
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid session"})
}

// setRefreshCookie is the single place the refresh cookie's attributes are
// written, so login and refresh cannot drift apart.
func setRefreshCookie(w http.ResponseWriter, value string) {
	//nolint:gosec // G124 false positive: Secure and HttpOnly are both set, and
	// SameSite is None BY DECISION (P2-D9), not by omission. The origins are
	// separate, so Lax or Strict would never send this cookie at all. Every
	// attribute is asserted one at a time by
	// TestLogin_SetsTheRefreshCookieWithEveryAttributeP2D9Requires, and
	// config.Load refuses to boot if the origins ever converge.
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    value,
		Path:     refreshCookiePath,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteNoneMode,
	})
}

// clearRefreshCookie tells the browser to delete the cookie.
//
// The Path has to match the one it was set with: a browser only replaces a
// cookie whose name AND path match, so clearing it at `/` would leave the real
// one in place and add a second, empty cookie beside it.
func clearRefreshCookie(w http.ResponseWriter) {
	//nolint:gosec // G124 false positive, same reasoning as setRefreshCookie.
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     refreshCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteNoneMode,
	})
}
