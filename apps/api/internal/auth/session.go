package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/gentleman/mascotapp/apps/api/internal/db/sqlcgen"
)

// Refresh-token rotation and reuse detection (identity-and-session / *Refresh
// tokens rotate on use and reuse revokes the whole family*, design P2-D2).
//
// This is the stolen-token containment story. An access token expires in
// fifteen minutes and nothing revokes it; the refresh token underneath is what
// carries revocation, and it is a long-lived bearer credential. The only thing
// standing between a stolen copy and a permanent session is this: the moment
// both copies are used, every token in the family dies.
//
// Which means the guarantee is NOT "we detect theft". It is "theft cannot
// outlive the legitimate user's next refresh" — whoever refreshes second is
// refused, and both are logged out. That is weaker than detection and stronger
// than nothing, and it is worth being precise about which one is on offer.
const (
	// RefreshSecretLength is the 32 random bytes P2-D2 fixes.
	//
	// The `user_id` prefix is attacker-chosen and the hash is a lookup key, so
	// this secret is the entire credential.
	RefreshSecretLength = 32

	// RefreshTokenLifetime is how long a session survives without being used.
	//
	// NOT INHERITED FROM THE SPEC — §5.2 fixes the access token's fifteen
	// minutes and says nothing about this one, so thirty days is a choice made
	// here. It is the trade between "volunteers log in again every week, so
	// they write the password on a sticky note" and "a stolen cookie is useful
	// for a month". Rotation is what makes the long end tolerable: each use
	// replaces the token, so a month-old session has a days-old credential.
	RefreshTokenLifetime = 30 * 24 * time.Hour

	// refreshCookieSeparator splits the two halves of the cookie value.
	refreshCookieSeparator = "."
)

// refreshEncoding is base64url without padding, for both halves of the cookie.
//
// Unpadded because `=` in a cookie value is legal but reliably mishandled
// somewhere in a proxy chain, and url-safe because `+` and `/` are not.
var refreshEncoding = base64.RawURLEncoding

// ErrRefreshTokenInvalid is returned for any cookie that cannot be rotated.
//
// One sentinel for "malformed", "never issued", "belongs to another user",
// "expired" and "already revoked". Every one of them is the same HTTP response,
// and telling them apart would tell an attacker how close they got.
var ErrRefreshTokenInvalid = errors.New("auth: the refresh token is not valid")

// ErrRefreshTokenReused marks the one case the server must be able to see.
//
// It WRAPS ErrRefreshTokenInvalid, so `errors.Is(err, ErrRefreshTokenInvalid)`
// still holds and no caller can mistake reuse for success. The distinction is
// for the server's own eyes -- a log line, an alert, a mail to the account --
// never for the response body. Reuse means a credential existed in two places,
// which is the difference between a bad cookie and evidence of theft.
var ErrRefreshTokenReused = fmt.Errorf("%w: it was already rotated, and its family has been "+
	"revoked", ErrRefreshTokenInvalid)

// RotationOutcome is what a rotation attempt decided, separately from whether
// the database worked.
//
// **This split is not ergonomic taste — it is required for correctness, and it
// was found by a failing test rather than reasoned about in advance.**
//
// `db.WithAuthUser` rolls the transaction back whenever its callback returns an
// error (`db/auth.go:68`, `:77`). Reuse detection is the one refusal in this
// system with a PERSISTENT side effect: it revokes the whole family. Returning
// `ErrRefreshTokenReused` from inside that callback rolled the revocation back
// with it — the thief was refused and kept a live session, which is precisely
// the outcome family revocation exists to prevent.
//
// So a refusal travels in this struct, and the callback returns nil, and the
// transaction commits carrying the revocation. Only an infrastructure failure —
// a broken connection, a query that will not run — is a real error, because
// that is the only case where undoing the work is right.
type RotationOutcome struct {
	// Cookie is the new value to set, empty unless the rotation succeeded.
	Cookie string

	// Refusal is why the caller must answer 401, or nil on success. It is
	// ErrRefreshTokenInvalid, or ErrRefreshTokenReused when the caller should
	// also raise an alarm.
	Refusal error
}

// RefreshCookie is a parsed cookie value.
type RefreshCookie struct {
	// UserID is what the CLIENT claims. It is never trusted as identity: its
	// only job is telling the caller which `WithAuthUser` scope to open, and
	// a client that lies scopes itself to rows that do not contain its hash.
	UserID uuid.UUID

	// Secret is the credential. `sha256(Secret)` is what the row holds.
	Secret []byte
}

// FormatRefreshCookie builds the value the browser stores:
// `<base64url(user_id)>.<base64url(secret)>`.
func FormatRefreshCookie(userID uuid.UUID, secret []byte) string {
	return refreshEncoding.EncodeToString(userID[:]) +
		refreshCookieSeparator +
		refreshEncoding.EncodeToString(secret)
}

// ParseRefreshCookie splits and decodes a cookie value.
//
// This is the first code an attacker reaches on the refresh path: it runs on
// unauthenticated input, before any hash is computed and before any transaction
// is opened. Everything it cannot fully account for is refused here rather than
// carried further in a partly-valid state.
func ParseRefreshCookie(raw string) (RefreshCookie, error) {
	// SplitN(…, 2) would accept a secret half containing separators. Exactly
	// two parts, or it is not a cookie this server issued.
	parts := strings.Split(raw, refreshCookieSeparator)
	if len(parts) != 2 {
		return RefreshCookie{}, fmt.Errorf("%w: the cookie is not two dot-separated parts",
			ErrRefreshTokenInvalid)
	}

	// RawURLEncoding refuses padding, which is what rejects a `=`-padded value
	// rather than silently accepting a second spelling of the same bytes.
	rawUser, err := refreshEncoding.DecodeString(parts[0])
	if err != nil {
		return RefreshCookie{}, fmt.Errorf("%w: the user half is not base64url",
			ErrRefreshTokenInvalid)
	}
	userID, err := uuid.FromBytes(rawUser)
	if err != nil {
		return RefreshCookie{}, fmt.Errorf("%w: the user half is not a uuid",
			ErrRefreshTokenInvalid)
	}

	secret, err := refreshEncoding.DecodeString(parts[1])
	if err != nil {
		return RefreshCookie{}, fmt.Errorf("%w: the secret half is not base64url",
			ErrRefreshTokenInvalid)
	}
	// A short secret is refused before it is hashed. SHA-256 happily digests
	// three bytes and produces a perfectly well-formed lookup key, so without
	// this the length check would never happen anywhere.
	if len(secret) != RefreshSecretLength {
		return RefreshCookie{}, fmt.Errorf("%w: the secret is %d bytes, want %d",
			ErrRefreshTokenInvalid, len(secret), RefreshSecretLength)
	}

	return RefreshCookie{UserID: userID, Secret: secret}, nil
}

// NewRefreshSecret draws one credential's worth of randomness.
func NewRefreshSecret() ([]byte, error) {
	secret := make([]byte, RefreshSecretLength)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("auth: drawing a refresh secret: %w", err)
	}

	return secret, nil
}

// HashRefreshSecret is what goes in `token_hash`.
//
// SHA-256 of the RAW BYTES, not of the cookie text and not of the base64 form.
// `token_hash` is UNIQUE and rotation finds the row by it, so these exact bytes
// are a contract between the issue path and the rotate path: hash the encoded
// form in one and the raw form in the other and you get two functions that each
// work alone and never find each other's rows.
//
// Unsalted and unstretched, like `totp_recovery_codes.code_hash` and for the
// same reason: 32 uniform random bytes have no low entropy to compensate for,
// and the lookup must find the row from the submitted secret alone.
func HashRefreshSecret(secret []byte) []byte {
	sum := sha256.Sum256(secret)

	return sum[:]
}

// IssueRefreshToken starts a NEW session family and returns its cookie value.
//
// This is the login path. Every login gets its own family, so revoking one
// device's session does not touch another's -- if login reused a family, a
// single stolen token would log the user out everywhere.
func IssueRefreshToken(
	ctx context.Context,
	tx pgx.Tx,
	userID uuid.UUID,
	now time.Time,
) (string, error) {
	return issueInFamily(ctx, tx, userID, uuid.New(), now)
}

// RotateRefreshToken spends the presented secret and returns a fresh cookie.
//
// **It takes the secret, never the parsed `user_id`.** The prefix is
// attacker-controlled input whose only job is telling the CALLER which
// `WithAuthUser` scope to open; handing it here too would give this function a
// second, weaker answer to a question RLS is already answering. The new token's
// owner comes off the row the database returned inside that scope, which is the
// one `user_id` no client chose.
//
// **The `error` return is infrastructure only.** A refused rotation comes back
// in RotationOutcome.Refusal with a nil error, so the caller commits — see
// RotationOutcome for why that is a correctness requirement and not a style
// choice. A caller that returns this function's error from its `WithAuthUser`
// callback is doing the right thing; a caller that returns `outcome.Refusal`
// from there would undo the family revocation.
func RotateRefreshToken(
	ctx context.Context,
	tx pgx.Tx,
	secret []byte,
	now time.Time,
) (RotationOutcome, error) {
	queries := sqlcgen.New(tx)

	presented, err := queries.GetRefreshTokenByHash(ctx, HashRefreshSecret(secret))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Either the secret was never issued, or the caller opened this
			// transaction under a different user and the RLS policy hid the
			// row. The two are indistinguishable here BY DESIGN -- and this is
			// P2-D2's stated limitation: a mangled `user_id` prefix is not
			// reported as reuse. Nothing was reused, the attacker got no
			// session, and an alarm on an attack that already failed is an
			// alarm people learn to ignore.
			return RotationOutcome{Refusal: ErrRefreshTokenInvalid}, nil
		}

		return RotationOutcome{}, fmt.Errorf(
			"auth: looking up the presented refresh token: %w", err)
	}

	// Everything above this line was a read that any number of transactions
	// could be doing at once. Everything below decides the fate of a family, so
	// from here the family is serialised.
	//
	// Judgment Day (T-02-021) found why this is not optional. Without it,
	// `RevokeRefreshTokenFamily` fixes its candidate rows at its own statement's
	// snapshot, and a successor inserted by a concurrent rotation afterwards
	// SURVIVES the revocation of its own family -- permanently, because nothing
	// revokes an already-revoked family twice. A partial unique index does not
	// help: that is an invariant WITHIN one transaction, and this is a race
	// BETWEEN two.
	if err := queries.LockRefreshTokenFamily(ctx, presented.FamilyID.String()); err != nil {
		return RotationOutcome{}, fmt.Errorf("auth: locking the token family: %w", err)
	}

	// NO re-read after the lock. An earlier version of this had one and mutation
	// testing killed it: removing it changed nothing observable, so it was not
	// load-bearing and it is gone.
	//
	// The reason it is safe to act on the pre-lock read: the only field that a
	// concurrent transaction can change is `revoked_at`, and a stale NULL there
	// is caught one statement later -- `RevokeRefreshTokenIfLive` carries
	// `revoked_at IS NULL`, so a family revoked while this transaction waited
	// yields zero rows and the rotation is refused. What is lost is only the
	// CLASSIFICATION: that refusal reads as "lost the race" rather than as
	// reuse. Benign, because the transaction that detected the reuse already
	// raised that alarm.

	// THE reuse check. A revoked row means this secret was already spent --
	// either rotated by its legitimate holder, or burned when its family was
	// revoked. Both mean a credential is in circulation that should not be, so
	// the family dies.
	//
	// Nothing in the database refuses a revoked token: the row is perfectly
	// selectable and `GetRefreshTokenByHash` carries no predicate beyond the
	// hash. This branch is the refusal, and the statement below is the
	// containment. Both are this package's, not the schema's.
	if presented.RevokedAt.Valid {
		if _, err := queries.RevokeRefreshTokenFamily(ctx, presented.FamilyID); err != nil {
			// The revocation failing is worse than the refusal: the thief
			// keeps a live token. It IS a real error — rolling back here is
			// correct, because nothing was contained.
			return RotationOutcome{}, fmt.Errorf(
				"auth: revoking the family of a reused refresh token: %w", err)
		}

		// nil error, so the caller COMMITS the revocation just written above.
		return RotationOutcome{Refusal: ErrRefreshTokenReused}, nil
	}

	// Expiry is refused WITHOUT touching the family. A session that simply sat
	// unused is not evidence of theft, and burning the family for it would log
	// the user out of every other device and fire the reuse alarm on a
	// non-event.
	if !presented.ExpiresAt.Valid || !presented.ExpiresAt.Time.After(now) {
		return RotationOutcome{Refusal: ErrRefreshTokenInvalid}, nil
	}

	// The presented token is revoked FIRST, before its successor is inserted
	// (00017): a unique index now enforces at most one live row per
	// family_id, and inserting the successor while the presented token was
	// still live would transiently hold two live rows in this family and be
	// rejected by that index.
	//
	// `revoked_at IS NULL` in this statement is what makes it the write that
	// loses a concurrent race: two simultaneous rotations of the same token
	// both read a live row above, and only one can affect a row here.
	revoked, err := queries.RevokeRefreshTokenIfLive(ctx, presented.ID)
	if err != nil {
		return RotationOutcome{}, fmt.Errorf(
			"auth: revoking the presented refresh token: %w", err)
	}
	if revoked == 0 {
		// Lost the race, and NOTHING has been written yet -- no successor
		// exists, so this is a real error and the caller's rollback has
		// nothing to undo. Refused, but NOT called reuse: the other winner
		// could be the same legitimate user losing a double-clicked refresh,
		// or a concurrent family revocation that got here first; alarming on
		// either would fire on a non-event.
		return RotationOutcome{}, fmt.Errorf("%w: another rotation of this token won the race",
			ErrRefreshTokenInvalid)
	}

	// The successor is inserted only now that the presented token is
	// revoked, so the family never transiently holds two live rows.
	fresh, freshID, err := issueInFamilyReturningID(ctx, tx,
		presented.UserID, presented.FamilyID, now)
	if err != nil {
		return RotationOutcome{}, err
	}

	// The back-pointer is set LAST because `replaced_by` is a foreign key
	// into this same table: the row it points at has to exist first. If
	// this fails, the caller's rollback undoes both the revoke and the
	// insert above.
	if err := queries.SetRefreshTokenReplacedBy(ctx, sqlcgen.SetRefreshTokenReplacedByParams{
		ID:         presented.ID,
		ReplacedBy: pgtype.UUID{Bytes: freshID, Valid: true},
	}); err != nil {
		return RotationOutcome{}, fmt.Errorf(
			"auth: pointing the presented refresh token at its successor: %w", err)
	}

	return RotationOutcome{Cookie: fresh}, nil
}

// issueInFamily writes one token into an existing or new family.
func issueInFamily(
	ctx context.Context,
	tx pgx.Tx,
	userID, familyID uuid.UUID,
	now time.Time,
) (string, error) {
	cookie, _, err := issueInFamilyReturningID(ctx, tx, userID, familyID, now)

	return cookie, err
}

// issueInFamilyReturningID is the shared write behind both paths. Rotation
// needs the new row's id for `replaced_by`; login does not.
func issueInFamilyReturningID(
	ctx context.Context,
	tx pgx.Tx,
	userID, familyID uuid.UUID,
	now time.Time,
) (string, uuid.UUID, error) {
	secret, err := NewRefreshSecret()
	if err != nil {
		return "", uuid.Nil, err
	}

	// v7: time-ordered, so a session's tokens land together in the primary
	// key's index instead of scattering across it.
	id, err := uuid.NewV7()
	if err != nil {
		return "", uuid.Nil, fmt.Errorf("auth: generating a refresh token id: %w", err)
	}

	err = sqlcgen.New(tx).InsertRefreshToken(ctx, sqlcgen.InsertRefreshTokenParams{
		ID:        id,
		UserID:    userID,
		TokenHash: HashRefreshSecret(secret),
		FamilyID:  familyID,
		ExpiresAt: pgtype.Timestamptz{Time: now.Add(RefreshTokenLifetime), Valid: true},
	})
	if err != nil {
		return "", uuid.Nil, fmt.Errorf("auth: storing a refresh token: %w", err)
	}

	// The cookie carries the row's OWN user, never a caller-supplied one.
	return FormatRefreshCookie(userID, secret), id, nil
}
