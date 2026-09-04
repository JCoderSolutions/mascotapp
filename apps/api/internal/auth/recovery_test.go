package auth_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gentleman/mascotapp/apps/api/internal/auth"
	"github.com/gentleman/mascotapp/apps/api/internal/db"
	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
)

// Recovery codes — the way back in when the authenticator app is on a phone
// that is gone (data-model-core / *TOTP recovery codes provide single-use
// account recovery, non-tenant scoped*, and design P2-D8).
//
// Ten codes of 128 random bits, shown exactly once, stored as SHA-256. NOT
// Argon2id, and the design says why: a password stretcher exists to compensate
// for low entropy, and there is none to compensate for here.
//
// This file decodes and re-hashes BY HAND — `encoding/base32` and
// `crypto/sha256` used directly, never a helper from the package under test.
// A test that hashes with the same function the implementation hashes with
// proves the function agrees with itself.

// The alphabet and the decoder are the test's own, not the implementation's.
var recoveryEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// -----------------------------------------------------------------------------
// Pure — generation and hashing. No container.
// -----------------------------------------------------------------------------

// THE case that separates "128 bits of entropy" from "a string that looks
// random". The code is decoded here, by this test, with a decoder the
// implementation never touches, and the byte count is checked against the
// number the design fixes. A generator emitting 64 bits, or 128 bits of a
// counter, would still produce ten distinct printable strings and pass every
// other test in this file.
func TestNewRecoveryCodes_ProducesTenDistinctCodesOfOneHundredTwentyEightBits(t *testing.T) {
	codes, err := auth.NewRecoveryCodes()
	if err != nil {
		t.Fatalf("generating a set of recovery codes: %v", err)
	}

	if len(codes) != auth.RecoveryCodeCount {
		t.Fatalf("got %d codes, want %d", len(codes), auth.RecoveryCodeCount)
	}
	if auth.RecoveryCodeCount != 10 {
		t.Fatalf("RecoveryCodeCount = %d, want 10 — the design fixes ten codes per set",
			auth.RecoveryCodeCount)
	}

	seen := make(map[string]struct{}, len(codes))
	for i, code := range codes {
		raw, err := recoveryEncoding.DecodeString(code.Plaintext)
		if err != nil {
			t.Fatalf("code %d (%q) is not base32 without padding: %v", i, code.Plaintext, err)
		}
		if len(raw) != 16 {
			t.Fatalf("code %d decodes to %d bytes, want 16 (128 bits)", i, len(raw))
		}

		// Two identical codes in one set would collide on `code_hash`, which
		// is UNIQUE: the insert of the tenth would fail and the user would be
		// handed a set that cannot be stored.
		if _, duplicate := seen[code.Plaintext]; duplicate {
			t.Fatalf("code %d repeats a code already in the same set", i)
		}
		seen[code.Plaintext] = struct{}{}
	}
}

// The anti-vacuity guard for the test above: everything there passes against a
// generator that returns the same ten hard-coded codes on every call.
func TestNewRecoveryCodes_ProducesADifferentSetEveryTime(t *testing.T) {
	first, err := auth.NewRecoveryCodes()
	if err != nil {
		t.Fatalf("generating the first set: %v", err)
	}
	second, err := auth.NewRecoveryCodes()
	if err != nil {
		t.Fatalf("generating the second set: %v", err)
	}

	inFirst := make(map[string]struct{}, len(first))
	for _, code := range first {
		inFirst[code.Plaintext] = struct{}{}
	}
	for i, code := range second {
		if _, repeated := inFirst[code.Plaintext]; repeated {
			t.Fatalf("code %d of the second set already appeared in the first — the "+
				"generator is not drawing fresh randomness", i)
		}
	}
}

// The hash is recomputed here from the plaintext, with `crypto/sha256`
// directly. If the implementation switched to SHA-1, to a salted scheme, or to
// Argon2id, this fails — and it should: `code_hash` is UNIQUE and a redemption
// looks the row up BY that hash, so the hash is a wire format between the
// enrolment path and the login path, not an implementation detail.
func TestHashRecoveryCode_IsSHA256OfThePlaintext(t *testing.T) {
	const plaintext = "JBSWY3DPEHPK3PXPJBSWY3DPEH"

	want := sha256.Sum256([]byte(plaintext))
	got := auth.HashRecoveryCode(plaintext)

	if !bytes.Equal(got, want[:]) {
		t.Fatalf("HashRecoveryCode(%q) = %x, want %x", plaintext, got, want[:])
	}

	// And the stored value is not the code itself — the scenario's own words.
	if bytes.Equal(got, []byte(plaintext)) {
		t.Fatal("the hash equals the plaintext code — nothing is hashed")
	}
}

// Each code carries its own hash. A generator that shuffled the pairing, or
// hashed the wrong element of the slice, hands the user ten codes none of which
// can ever be redeemed — every lookup misses, and the failure surfaces months
// later when somebody actually needs one.
func TestNewRecoveryCodes_PairsEachCodeWithItsOwnHash(t *testing.T) {
	codes, err := auth.NewRecoveryCodes()
	if err != nil {
		t.Fatalf("generating a set of recovery codes: %v", err)
	}

	for i, code := range codes {
		want := sha256.Sum256([]byte(code.Plaintext))
		if !bytes.Equal(code.Hash, want[:]) {
			t.Fatalf("code %d carries hash %x, but its own plaintext hashes to %x",
				i, code.Hash, want[:])
		}
	}
}

// -----------------------------------------------------------------------------
// Integration — the database side. Container.
// -----------------------------------------------------------------------------

// Regeneration replaces the set: all ten of the old rows go, ten new ones
// arrive, and it happens inside the one transaction the caller opened. The
// design's words: "a code is marked used, never deleted individually --
// regenerating the set deletes all ten and inserts ten more, in one
// transaction".
func TestRegenerateRecoveryCodes_ReplacesTheWholeSet(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	user := uuid.New()
	seedRecoveryUser(ctx, t, env.OwnerPool, user)

	old, err := auth.NewRecoveryCodes()
	if err != nil {
		t.Fatalf("generating the first set: %v", err)
	}
	if err := withUser(ctx, env, user, func(ctx context.Context, tx pgx.Tx) error {
		return auth.RegenerateRecoveryCodes(ctx, tx, user, old)
	}); err != nil {
		t.Fatalf("writing the first set: %v", err)
	}

	fresh, err := auth.NewRecoveryCodes()
	if err != nil {
		t.Fatalf("generating the replacement set: %v", err)
	}
	if err := withUser(ctx, env, user, func(ctx context.Context, tx pgx.Tx) error {
		return auth.RegenerateRecoveryCodes(ctx, tx, user, fresh)
	}); err != nil {
		t.Fatalf("writing the replacement set: %v", err)
	}

	stored := storedHashes(ctx, t, env, user)

	if len(stored) != auth.RecoveryCodeCount {
		t.Fatalf("the user holds %d codes after regenerating, want %d — the old set was "+
			"not cleared, or the new one was not written in full",
			len(stored), auth.RecoveryCodeCount)
	}
	for i, code := range old {
		if _, survives := stored[string(code.Hash)]; survives {
			t.Fatalf("code %d of the SUPERSEDED set is still redeemable after regeneration",
				i)
		}
	}
	for i, code := range fresh {
		if _, present := stored[string(code.Hash)]; !present {
			t.Fatalf("code %d of the new set was not stored", i)
		}
	}
}

// THE positive probe, and the one test here that dies if the hashing is wrong.
//
// Every refusal test below stays red when `RedeemRecoveryCode` hashes the
// submitted code incorrectly: the lookup simply misses and the code is refused,
// which is what those tests assert. Only a redemption that must SUCCEED can
// tell "hashed correctly and matched" apart from "hashed wrongly and missed".
//
// It also pins that the row is MARKED, not deleted. The grant is
// `UPDATE (used_at)` plus a table-wide `DELETE`, so nothing at the database
// level stops this package from deleting a redeemed row — the audit trail of
// which code was used, and when, is a decision this layer has to make and keep.
func TestRedeemRecoveryCode_AcceptsAValidCodeAndMarksItWithoutDeletingIt(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	user := uuid.New()
	seedRecoveryUser(ctx, t, env.OwnerPool, user)

	codes, err := auth.NewRecoveryCodes()
	if err != nil {
		t.Fatalf("generating a set of recovery codes: %v", err)
	}
	if err := withUser(ctx, env, user, func(ctx context.Context, tx pgx.Tx) error {
		return auth.RegenerateRecoveryCodes(ctx, tx, user, codes)
	}); err != nil {
		t.Fatalf("writing the set: %v", err)
	}

	redeemed := codes[3]
	if err := withUser(ctx, env, user, func(ctx context.Context, tx pgx.Tx) error {
		return auth.RedeemRecoveryCode(ctx, tx, redeemed.Plaintext)
	}); err != nil {
		t.Fatalf("redeeming a valid, unused code: %v — a user locked out of their "+
			"authenticator cannot get back in", err)
	}

	// The row is still there. Ten rows, not nine.
	stored := storedHashes(ctx, t, env, user)
	if len(stored) != auth.RecoveryCodeCount {
		t.Fatalf("the user holds %d codes after redeeming one, want %d — the redeemed row "+
			"was deleted instead of marked", len(stored), auth.RecoveryCodeCount)
	}

	usedAt, found := stored[string(redeemed.Hash)]
	if !found {
		t.Fatal("the redeemed code's row is gone")
	}
	if !usedAt {
		t.Fatal("the redeemed code's `used_at` is still null")
	}

	// And redeeming one code did not consume the other nine.
	for i, code := range codes {
		if i == 3 {
			continue
		}
		if marked := stored[string(code.Hash)]; marked {
			t.Fatalf("code %d was marked used too — one redemption burned the whole set", i)
		}
	}
}

// A second redemption of the same code is refused.
//
// **Which layer answers, stated on purpose:** the refusal comes from the SQL
// predicate `used_at IS NULL`, which `TestTOTPRecoveryCodes_RedemptionIsSingleUse`
// in `rlstest` already pins at the table level. What THIS test adds, and the
// only thing it can claim, is the translation: this package must turn "the
// statement succeeded and affected zero rows" into `ErrRecoveryCodeInvalid`
// rather than into a nil error. A pass-through that returned nil on zero rows
// would let a used code complete a login while the table behaved perfectly.
func TestRedeemRecoveryCode_RefusesASecondRedemptionOfTheSameCode(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	user := uuid.New()
	seedRecoveryUser(ctx, t, env.OwnerPool, user)

	codes, err := auth.NewRecoveryCodes()
	if err != nil {
		t.Fatalf("generating a set of recovery codes: %v", err)
	}
	if err := withUser(ctx, env, user, func(ctx context.Context, tx pgx.Tx) error {
		return auth.RegenerateRecoveryCodes(ctx, tx, user, codes)
	}); err != nil {
		t.Fatalf("writing the set: %v", err)
	}

	if err := withUser(ctx, env, user, func(ctx context.Context, tx pgx.Tx) error {
		return auth.RedeemRecoveryCode(ctx, tx, codes[0].Plaintext)
	}); err != nil {
		t.Fatalf("the first redemption should have succeeded: %v", err)
	}

	err = withUser(ctx, env, user, func(ctx context.Context, tx pgx.Tx) error {
		return auth.RedeemRecoveryCode(ctx, tx, codes[0].Plaintext)
	})
	if !errors.Is(err, auth.ErrRecoveryCodeInvalid) {
		t.Fatalf("the second redemption returned %v, want ErrRecoveryCodeInvalid", err)
	}
}

// A code that was never issued is refused, and with the SAME sentinel a used
// code gets.
//
// One error for both is deliberate. Telling the caller "that code does not
// exist" apart from "that code is already spent" tells an attacker whether they
// guessed a real code — and no legitimate caller does anything different with
// the distinction: both are a failed login.
func TestRedeemRecoveryCode_RefusesACodeThatWasNeverIssued(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	user := uuid.New()
	seedRecoveryUser(ctx, t, env.OwnerPool, user)

	codes, err := auth.NewRecoveryCodes()
	if err != nil {
		t.Fatalf("generating a set of recovery codes: %v", err)
	}
	if err := withUser(ctx, env, user, func(ctx context.Context, tx pgx.Tx) error {
		return auth.RegenerateRecoveryCodes(ctx, tx, user, codes)
	}); err != nil {
		t.Fatalf("writing the set: %v", err)
	}

	// A well-formed code from a different draw — the shape is right, the value
	// was never stored for anybody.
	stranger, err := auth.NewRecoveryCodes()
	if err != nil {
		t.Fatalf("generating the stranger's set: %v", err)
	}

	err = withUser(ctx, env, user, func(ctx context.Context, tx pgx.Tx) error {
		return auth.RedeemRecoveryCode(ctx, tx, stranger[0].Plaintext)
	})
	if !errors.Is(err, auth.ErrRecoveryCodeInvalid) {
		t.Fatalf("redeeming an unissued code returned %v, want ErrRecoveryCodeInvalid", err)
	}

	// And nothing of the real user's was touched on the way past.
	stored := storedHashes(ctx, t, env, user)
	for i, code := range codes {
		if marked := stored[string(code.Hash)]; marked {
			t.Fatalf("code %d was marked used by a failed redemption of somebody else's code",
				i)
		}
	}
}

// One user cannot redeem another user's code.
//
// **Which layer answers, again stated:** the RLS policy
// `USING (user_id = app.user_id)` is what hides the row, not anything in this
// package — `RedeemRecoveryCode`'s statement carries no `user_id` at all, by
// design, because the transaction is already scoped by `WithAuthUser`. What
// this test pins is that the scoping is not bypassed here: a future version
// that reached for `env.OwnerPool`, or that added its own `user_id` parameter
// and got it from the request, would break exactly this case.
func TestRedeemRecoveryCode_RefusesAnotherUsersCode(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	owner := uuid.New()
	intruder := uuid.New()
	seedRecoveryUser(ctx, t, env.OwnerPool, owner)
	seedRecoveryUser(ctx, t, env.OwnerPool, intruder)

	codes, err := auth.NewRecoveryCodes()
	if err != nil {
		t.Fatalf("generating the owner's codes: %v", err)
	}
	if err := withUser(ctx, env, owner, func(ctx context.Context, tx pgx.Tx) error {
		return auth.RegenerateRecoveryCodes(ctx, tx, owner, codes)
	}); err != nil {
		t.Fatalf("writing the owner's set: %v", err)
	}

	err = withUser(ctx, env, intruder, func(ctx context.Context, tx pgx.Tx) error {
		return auth.RedeemRecoveryCode(ctx, tx, codes[0].Plaintext)
	})
	if !errors.Is(err, auth.ErrRecoveryCodeInvalid) {
		t.Fatalf("the intruder's redemption returned %v, want ErrRecoveryCodeInvalid", err)
	}

	// The owner's code is still unused and still theirs to redeem.
	stored := storedHashes(ctx, t, env, owner)
	if marked := stored[string(codes[0].Hash)]; marked {
		t.Fatal("the intruder's attempt marked the owner's code as used")
	}
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

// withUser runs fn inside the auth-scoped transaction the production callers
// use. This package never opens its own: `SET LOCAL app.user_id` and the
// transaction boundary belong to `db.WithAuthUser`, and a function here that
// took a pool instead of a `pgx.Tx` would be free to run outside that scope.
func withUser(
	ctx context.Context,
	env *dbtest.Env,
	user uuid.UUID,
	fn func(context.Context, pgx.Tx) error,
) error {
	return db.WithAuthUser(ctx, env.AuthPool, user, fn)
}

// seedRecoveryUser creates the user as the OWNER role. `app_auth` has no grant
// that would let it create a user, and it should not: the identity door reads
// and updates credentials, it does not mint accounts.
//
// `ctx` leads and `t` follows, against the usual test-helper habit: revive's
// `context-as-argument` wants the context first, and whether its default
// exemption covers `*testing.T` depends on a version this file should not have
// an opinion about.
func seedRecoveryUser(ctx context.Context, t *testing.T, owner *pgxpool.Pool, id uuid.UUID) {
	t.Helper()

	_, err := owner.Exec(ctx,
		`INSERT INTO users (id, email, full_name) VALUES ($1, $2, $3)`,
		id, fmt.Sprintf("recovery-%s@example.org", id), "A Recovery User")
	if err != nil {
		t.Fatalf("seeding user %s: %v", id, err)
	}
}

// storedHashes reads the user's rows back and returns hash → "is it marked
// used". It reads with raw SQL through the auth door rather than through
// anything in `auth`, so the assertions above are checking the database's
// contents, not the package's opinion of them.
func storedHashes(
	ctx context.Context,
	t *testing.T,
	env *dbtest.Env,
	user uuid.UUID,
) map[string]bool {
	t.Helper()

	stored := make(map[string]bool, auth.RecoveryCodeCount)
	err := withUser(ctx, env, user, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT code_hash, used_at IS NOT NULL FROM totp_recovery_codes WHERE user_id = $1`,
			user)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var hash []byte
			var used bool
			if err := rows.Scan(&hash, &used); err != nil {
				return err
			}
			stored[string(hash)] = used
		}

		return rows.Err()
	})
	if err != nil {
		t.Fatalf("reading back the stored codes: %v", err)
	}

	return stored
}
