package auth_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gentleman/mascotapp/apps/api/internal/auth"
	"github.com/gentleman/mascotapp/apps/api/internal/db"
	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
)

// Refresh-token rotation and reuse detection (identity-and-session / *Refresh
// tokens rotate on use and reuse revokes the whole family*, design P2-D2).
//
// This is the stolen-token containment story, and the reason it is the phase's
// Judgment Day subject: a refresh token is a long-lived bearer credential, and
// the ONLY thing standing between a stolen one and a permanent session is that
// the moment both copies are used, the family dies.
//
// The cookie value is `<base64url(user_id)>.<base64url(secret)>` — 32 random
// bytes of secret, `sha256(secret)` in `token_hash`. The prefix exists because
// the row must be found by hash BEFORE the user is known, and a user-scoped RLS
// policy cannot come from an identity nobody has yet.
//
// This file decodes that value BY HAND with `encoding/base64` and re-hashes
// with `crypto/sha256`, never through a helper of the package under test.

// The decoder is the test's own.
var refreshEncoding = base64.RawURLEncoding

// -----------------------------------------------------------------------------
// Pure — the cookie value and its parts. No container.
// -----------------------------------------------------------------------------

// The wire format, pinned from outside. `FormatRefreshCookie` is not asked to
// agree with `ParseRefreshCookie`; it is asked to produce the exact two-part
// value P2-D2 specifies, because the browser stores this string and a future
// version that changed the separator or the alphabet would log every session
// out with no test noticing.
func TestFormatRefreshCookie_IsTheUserIDAndSecretJoinedByADot(t *testing.T) {
	user := uuid.MustParse("6f1b7c62-0f1a-4f4e-9f3b-2c5d8e7a1b09")
	secret := bytes.Repeat([]byte{0xAB}, auth.RefreshSecretLength)

	value := auth.FormatRefreshCookie(user, secret)

	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		t.Fatalf("the cookie value split into %d parts on '.', want 2: %q", len(parts), value)
	}

	rawUser, err := refreshEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("the user_id half is not base64url without padding: %v", err)
	}
	gotUser, err := uuid.FromBytes(rawUser)
	if err != nil {
		t.Fatalf("the user_id half does not decode to a uuid: %v", err)
	}
	if gotUser != user {
		t.Fatalf("the cookie carries user %s, want %s", gotUser, user)
	}

	rawSecret, err := refreshEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("the secret half is not base64url without padding: %v", err)
	}
	if !bytes.Equal(rawSecret, secret) {
		t.Fatalf("the cookie carries secret %x, want %x", rawSecret, secret)
	}
}

// Everything a client can send that is not a cookie this server issued.
//
// The parser runs on unauthenticated input from a browser — it is the first
// code an attacker reaches on the refresh path, before any hash is computed and
// before any transaction is opened.
func TestParseRefreshCookie_RefusesMalformedValues(t *testing.T) {
	user := uuid.New()
	goodUser := refreshEncoding.EncodeToString(userBytes(t, user))
	goodSecret := refreshEncoding.EncodeToString(bytes.Repeat([]byte{0x01}, auth.RefreshSecretLength))

	cases := map[string]string{
		"empty":                   "",
		"no separator":            goodUser + goodSecret,
		"two separators":          goodUser + "." + goodSecret + "." + goodSecret,
		"empty user half":         "." + goodSecret,
		"empty secret half":       goodUser + ".",
		"user half is not base64": "not base64!!" + "." + goodSecret,
		"secret half is not b64":  goodUser + "." + "not base64!!",
		"user half is not a uuid": refreshEncoding.EncodeToString([]byte{0x01, 0x02}) + "." + goodSecret,
		"secret is short":         goodUser + "." + refreshEncoding.EncodeToString([]byte{0x01, 0x02, 0x03}),
		"separator only":          ".",
		"padded base64":           base64.URLEncoding.EncodeToString(userBytes(t, user)) + "." + goodSecret,
	}

	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := auth.ParseRefreshCookie(raw); err == nil {
				t.Fatalf("ParseRefreshCookie(%q) was accepted", raw)
			}
		})
	}
}

// The anti-vacuity guard for the table above: a parser that refused everything
// would pass all eleven cases and reject every real session.
func TestParseRefreshCookie_AcceptsACookieThisServerWouldIssue(t *testing.T) {
	user := uuid.New()
	secret := bytes.Repeat([]byte{0x7F}, auth.RefreshSecretLength)

	parsed, err := auth.ParseRefreshCookie(auth.FormatRefreshCookie(user, secret))
	if err != nil {
		t.Fatalf("a cookie this server formats was refused by its own parser: %v", err)
	}
	if parsed.UserID != user {
		t.Fatalf("parsed user %s, want %s", parsed.UserID, user)
	}
	if !bytes.Equal(parsed.Secret, secret) {
		t.Fatalf("parsed secret %x, want %x", parsed.Secret, secret)
	}
}

// 32 bytes, and a different 32 every time.
//
// The secret is the entire credential: the `user_id` prefix is attacker-chosen
// and the hash is a lookup key, so guessing this is guessing the session.
func TestNewRefreshSecret_Is32FreshRandomBytes(t *testing.T) {
	if auth.RefreshSecretLength != 32 {
		t.Fatalf("RefreshSecretLength = %d, want 32 — P2-D2 fixes 32 random bytes",
			auth.RefreshSecretLength)
	}

	seen := make(map[string]struct{}, 64)
	for i := range 64 {
		secret, err := auth.NewRefreshSecret()
		if err != nil {
			t.Fatalf("drawing secret %d: %v", i, err)
		}
		if len(secret) != auth.RefreshSecretLength {
			t.Fatalf("secret %d is %d bytes, want %d", i, len(secret), auth.RefreshSecretLength)
		}
		if _, repeated := seen[string(secret)]; repeated {
			t.Fatalf("secret %d repeats an earlier draw — this is not fresh randomness", i)
		}
		seen[string(secret)] = struct{}{}
	}
}

// The hash is recomputed here, and it is the hash of the SECRET BYTES — not of
// the cookie string, and not of the secret's base64 text.
//
// This matters more than it looks: `token_hash` is UNIQUE and the rotation
// lookup finds the row by it, so the exact bytes fed to SHA-256 are a contract
// between the issue path and the rotate path. Hashing the encoded form in one
// and the raw form in the other produces two functions that each work alone and
// never find each other's rows.
func TestHashRefreshSecret_IsSHA256OfTheRawSecretBytes(t *testing.T) {
	secret := bytes.Repeat([]byte{0x2A}, auth.RefreshSecretLength)

	want := sha256.Sum256(secret)
	got := auth.HashRefreshSecret(secret)

	if !bytes.Equal(got, want[:]) {
		t.Fatalf("HashRefreshSecret = %x, want %x", got, want[:])
	}

	encoded := sha256.Sum256([]byte(refreshEncoding.EncodeToString(secret)))
	if bytes.Equal(got, encoded[:]) {
		t.Fatal("the hash is of the secret's base64 TEXT, not of its bytes")
	}
}

// -----------------------------------------------------------------------------
// Integration — rotation and reuse against the real table. Container.
// -----------------------------------------------------------------------------

// Every issue starts its own family, so one device's session dying does not
// take another's with it. Family-wide revocation is the containment mechanism;
// if login reused a family, a single stolen token would log the user out
// everywhere on every device.
func TestIssueRefreshToken_StartsAFamilyOfItsOwn(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()
	now := time.Now()

	user := uuid.New()
	seedSessionUser(ctx, t, env, user)

	first := issue(ctx, t, env, user, now)
	second := issue(ctx, t, env, user, now)

	firstRow := readSession(ctx, t, env, user, first)
	secondRow := readSession(ctx, t, env, user, second)

	if firstRow.familyID == secondRow.familyID {
		t.Fatalf("two logins share family %s — revoking one would revoke the other",
			firstRow.familyID)
	}
	if firstRow.expiresAt.Before(now) || secondRow.expiresAt.Before(now) {
		t.Fatal("a freshly issued token is already expired")
	}
}

// Scenario: A valid, not-yet-rotated refresh token rotates successfully.
func TestRotateRefreshToken_IssuesANewTokenInTheSameFamilyAndMarksTheOldOne(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()
	now := time.Now()

	user := uuid.New()
	seedSessionUser(ctx, t, env, user)

	old := issue(ctx, t, env, user, now)
	oldRow := readSession(ctx, t, env, user, old)

	fresh, err := rotate(ctx, env, user, old, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("rotating a valid, not-yet-rotated token: %v", err)
	}
	if fresh == old {
		t.Fatal("rotation returned the presented cookie unchanged — nothing rotated")
	}

	freshRow := readSession(ctx, t, env, user, fresh)
	if freshRow.familyID != oldRow.familyID {
		t.Fatalf("the new token is in family %s, want %s — rotation started a new family "+
			"instead of continuing the session's", freshRow.familyID, oldRow.familyID)
	}
	if freshRow.revokedAt != nil {
		t.Fatal("the newly issued token is already revoked")
	}

	// The presented token is marked rotated: `replaced_by` points at the new
	// row, and it is no longer usable.
	oldAfter := readSession(ctx, t, env, user, old)
	if oldAfter.replacedBy == nil {
		t.Fatal("the presented token's `replaced_by` is still null — it was not marked rotated")
	}
	if *oldAfter.replacedBy != freshRow.id {
		t.Fatalf("`replaced_by` points at %s, want the new token %s",
			*oldAfter.replacedBy, freshRow.id)
	}
	if oldAfter.revokedAt == nil {
		t.Fatal("the presented token is still live after being rotated")
	}
}

// Scenario: Reusing an already-rotated token revokes its entire family.
//
// **Which layer answers.** The refusal is this package's: it reads
// `revoked_at` off the row and decides. Nothing in the database refuses a
// revoked token — the row is perfectly selectable, and `GetRefreshTokenByHash`
// carries no predicate beyond the hash. The family revocation is likewise this
// package's decision to issue that statement at all; the SQL only executes it.
func TestRotateRefreshToken_ReusingARotatedTokenRevokesTheWholeFamily(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()
	now := time.Now()

	user := uuid.New()
	seedSessionUser(ctx, t, env, user)

	stolen := issue(ctx, t, env, user, now)
	live, err := rotate(ctx, env, user, stolen, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("the legitimate rotation should have succeeded: %v", err)
	}

	// The thief presents the copy they took before the legitimate rotation.
	_, err = rotate(ctx, env, user, stolen, now.Add(2*time.Minute))
	if !errors.Is(err, auth.ErrRefreshTokenInvalid) {
		t.Fatalf("reusing a rotated token returned %v, want ErrRefreshTokenInvalid", err)
	}
	// Reuse is a security EVENT, not merely a refusal: the caller has to be
	// able to log it apart from an ordinary bad cookie. It still satisfies the
	// invalid sentinel above, so no caller can accidentally treat it as
	// success.
	if !errors.Is(err, auth.ErrRefreshTokenReused) {
		t.Fatalf("reusing a rotated token returned %v, want it to also be "+
			"ErrRefreshTokenReused so the event can be alarmed on", err)
	}

	// THE assertion: the token the LEGITIMATE user is holding right now — which
	// was valid a moment ago and which the thief never touched — is revoked.
	liveRow := readSession(ctx, t, env, user, live)
	if liveRow.revokedAt == nil {
		t.Fatal("the currently valid token survived the reuse event — the family was not " +
			"revoked, only the presented token, and the thief keeps their session")
	}
}

// Scenario: A token from a revoked family is refused after the reuse event.
//
// This is the scenario the one above cannot prove on its own. Marking a row
// `revoked_at` and actually refusing it are two different things, and a
// rotation path that only checked `replaced_by` would leave the still-unrotated
// token working after its family had been burned.
func TestRotateRefreshToken_ATokenValidBeforeTheReuseEventIsRefusedAfterIt(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()
	now := time.Now()

	user := uuid.New()
	seedSessionUser(ctx, t, env, user)

	stolen := issue(ctx, t, env, user, now)
	live, err := rotate(ctx, env, user, stolen, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("the legitimate rotation should have succeeded: %v", err)
	}

	// Prove `live` WAS usable before the reuse event, or the assertion after it
	// proves nothing. Rotating it here would consume it, so this only checks
	// the row the previous test already exercised end to end.
	if readSession(ctx, t, env, user, live).revokedAt != nil {
		t.Fatal("the live token was already revoked before the reuse event")
	}

	if _, err := rotate(ctx, env, user, stolen, now.Add(2*time.Minute)); err == nil {
		t.Fatal("the reuse was not detected, so this test cannot measure its consequence")
	}

	_, err = rotate(ctx, env, user, live, now.Add(3*time.Minute))
	if !errors.Is(err, auth.ErrRefreshTokenInvalid) {
		t.Fatalf("the token that was valid before the reuse event returned %v after it, "+
			"want ErrRefreshTokenInvalid — the revocation was single-token, not family-wide",
			err)
	}
}

// P2-D2's honest limitation, written as a test so it stays honest.
//
// **Which layer answers: RLS.** A client that lies about the `user_id` prefix
// scopes itself to a user whose rows do not contain that hash. The policy
// `user_id = app.user_id` hides the row, `GetRefreshTokenByHash` returns
// nothing, and the request is refused — by the database, not by this package.
//
// And it is deliberately NOT reported as reuse. The design says so and this
// pins it: nothing was reused, the attacker got no session, and alarming on an
// attack that already failed trains people to ignore the alarm.
func TestRotateRefreshToken_RefusesAMangledUserPrefixWithoutCallingItReuse(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()
	now := time.Now()

	owner := uuid.New()
	stranger := uuid.New()
	seedSessionUser(ctx, t, env, owner)
	seedSessionUser(ctx, t, env, stranger)

	cookie := issue(ctx, t, env, owner, now)
	parsed, err := auth.ParseRefreshCookie(cookie)
	if err != nil {
		t.Fatalf("parsing the cookie this server issued: %v", err)
	}

	// The real secret, presented under somebody else's scope.
	_, err = rotate(ctx, env, stranger, auth.FormatRefreshCookie(stranger, parsed.Secret),
		now.Add(time.Minute))
	if !errors.Is(err, auth.ErrRefreshTokenInvalid) {
		t.Fatalf("a mangled prefix returned %v, want ErrRefreshTokenInvalid", err)
	}
	if errors.Is(err, auth.ErrRefreshTokenReused) {
		t.Fatal("a mangled prefix was reported as reuse — P2-D2 states it is not, and an " +
			"alarm on an attack that already failed is an alarm people learn to ignore")
	}

	// And the owner's session is untouched: a stranger must not be able to burn
	// somebody else's family by guessing at prefixes.
	if readSession(ctx, t, env, owner, cookie).revokedAt != nil {
		t.Fatal("the owner's token was revoked by a stranger's failed attempt")
	}
}

// An expired token is refused, and its family SURVIVES.
//
// Expiry is not evidence of theft — it is the ordinary end of a session that
// sat unused. Burning the family for it would log a user out of every device
// because one of them went quiet, and would fire the reuse alarm on a
// non-event.
func TestRotateRefreshToken_RefusesAnExpiredTokenWithoutBurningItsFamily(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()
	now := time.Now()

	user := uuid.New()
	seedSessionUser(ctx, t, env, user)

	// Issued far enough in the past that it is expired by `now`.
	stale := issue(ctx, t, env, user, now.Add(-2*auth.RefreshTokenLifetime))

	_, err := rotate(ctx, env, user, stale, now)
	if !errors.Is(err, auth.ErrRefreshTokenInvalid) {
		t.Fatalf("an expired token returned %v, want ErrRefreshTokenInvalid", err)
	}
	if errors.Is(err, auth.ErrRefreshTokenReused) {
		t.Fatal("an expired token was reported as reuse — it was never used twice")
	}

	if readSession(ctx, t, env, user, stale).revokedAt != nil {
		t.Fatal("expiry revoked the row; a session that simply timed out is not a theft, " +
			"and this would burn every other device in the family")
	}
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

// sessionRow is what the test reads back with raw SQL, so the assertions above
// check the table's contents rather than the package's opinion of them.
type sessionRow struct {
	id         uuid.UUID
	familyID   uuid.UUID
	expiresAt  time.Time
	revokedAt  *time.Time
	replacedBy *uuid.UUID
}

// issue puts one token in the database through the production path and returns
// its cookie value.
func issue(
	ctx context.Context,
	t *testing.T,
	env *dbtest.Env,
	user uuid.UUID,
	now time.Time,
) string {
	t.Helper()

	var cookie string
	err := db.WithAuthUser(ctx, env.AuthPool, user, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		cookie, err = auth.IssueRefreshToken(ctx, tx, user, now)

		return err
	})
	if err != nil {
		t.Fatalf("issuing a refresh token for %s: %v", user, err)
	}

	return cookie
}

// rotate presents a cookie under `scope`'s auth transaction.
//
// `scope` is a parameter rather than derived from the cookie because the whole
// point of the mangled-prefix test is that the caller opens the transaction
// with what the CLIENT claimed, not with the truth.
func rotate(
	ctx context.Context,
	env *dbtest.Env,
	scope uuid.UUID,
	cookie string,
	now time.Time,
) (string, error) {
	parsed, err := auth.ParseRefreshCookie(cookie)
	if err != nil {
		return "", err
	}

	var fresh string
	err = db.WithAuthUser(ctx, env.AuthPool, scope, func(ctx context.Context, tx pgx.Tx) error {
		fresh, err = auth.RotateRefreshToken(ctx, tx, parsed.Secret, now)

		return err
	})

	return fresh, err
}

// readSession reads the row behind a cookie with raw SQL, hashing the secret
// here rather than through the package under test.
func readSession(
	ctx context.Context,
	t *testing.T,
	env *dbtest.Env,
	scope uuid.UUID,
	cookie string,
) sessionRow {
	t.Helper()

	parsed, err := auth.ParseRefreshCookie(cookie)
	if err != nil {
		t.Fatalf("parsing a cookie this server issued: %v", err)
	}
	hash := sha256.Sum256(parsed.Secret)

	var row sessionRow
	err = db.WithAuthUser(ctx, env.AuthPool, scope, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT id, family_id, expires_at, revoked_at, replaced_by
			   FROM refresh_tokens WHERE token_hash = $1`, hash[:]).
			Scan(&row.id, &row.familyID, &row.expiresAt, &row.revokedAt, &row.replacedBy)
	})
	if err != nil {
		t.Fatalf("reading the session row back: %v", err)
	}

	return row
}

// seedSessionUser creates the user as the OWNER role: `app_auth` reads and
// updates credentials, it does not mint accounts.
func seedSessionUser(ctx context.Context, t *testing.T, env *dbtest.Env, id uuid.UUID) {
	t.Helper()

	_, err := env.OwnerPool.Exec(ctx,
		`INSERT INTO users (id, email, full_name) VALUES ($1, $2, $3)`,
		id, fmt.Sprintf("session-%s@example.org", id), "A Session User")
	if err != nil {
		t.Fatalf("seeding user %s: %v", id, err)
	}
}

// userBytes is the uuid's 16 raw bytes, for building cookie halves by hand.
func userBytes(t *testing.T, id uuid.UUID) []byte {
	t.Helper()

	raw, err := id.MarshalBinary()
	if err != nil {
		t.Fatalf("marshalling %s: %v", id, err)
	}

	return raw
}
