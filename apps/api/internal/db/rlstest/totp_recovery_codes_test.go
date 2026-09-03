package rlstest_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gentleman/mascotapp/apps/api/internal/db"
	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
)

// totp_recovery_codes (P2-D8, migration 00014): single-use account recovery,
// reachable only through app_auth and scoped to the caller's own rows by
// user_id -- the same door and the same predicate shape as refresh_tokens in
// auth_door_test.go, for a table that stores a different kind of secret.
//
// The two cases below are the data-model-core scenarios verbatim: codes are
// hashed, never plaintext, and a used code cannot be redeemed twice.

// recoveryCodeHash stands in for the real hashing recovery.go (T-02-017/018)
// will do -- SHA-256, per P2-D8's "a password stretcher exists to compensate
// for low entropy, and there is none to compensate for here". This test does
// not exercise that package; it proves the TABLE holds a hash and rejects a
// second redemption, independent of whoever computes it.
func recoveryCodeHash(plaintext string) []byte {
	sum := sha256.Sum256([]byte(plaintext))

	return sum[:]
}

// Scenario: Recovery codes are stored hashed, not plaintext.
func TestTOTPRecoveryCodes_StoredHashedNotPlaintext(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	user := uuid.New()
	if err := seedMemberUser(ctx, env.OwnerPool, user); err != nil {
		t.Fatalf("seeding the user: %v", err)
	}

	plaintext := "recovery-code-" + uuid.NewString()
	hash := recoveryCodeHash(plaintext)
	id := uuid.New()

	err := db.WithAuthUser(ctx, env.AuthPool, user,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO totp_recovery_codes (id, user_id, code_hash) VALUES ($1, $2, $3)`,
				id, user, hash)

			return err
		})
	if err != nil {
		t.Fatalf("app_auth could not insert a recovery code in its own scope: %v", err)
	}

	var stored []byte
	err = db.WithAuthUser(ctx, env.AuthPool, user,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				`SELECT code_hash FROM totp_recovery_codes WHERE id = $1`, id).Scan(&stored)
		})
	if err != nil {
		t.Fatalf("app_auth could not read back its own recovery code: %v", err)
	}

	// The row does not hold the plaintext: the stored bytes are the hash, not
	// the code a user would type in.
	if bytes.Equal(stored, []byte(plaintext)) {
		t.Fatalf("the stored code_hash equals the plaintext code — nothing is hashed")
	}

	// A submitted code can still be verified by comparing its hash: the same
	// deterministic hash of the plaintext must equal what is stored.
	if !bytes.Equal(stored, hash) {
		t.Fatalf("code_hash = %x, want %x — a submitted code could not be verified against it",
			stored, hash)
	}
}

// Scenario: A used recovery code is marked and a second redemption is
// rejected.
func TestTOTPRecoveryCodes_RedemptionIsSingleUse(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	user := uuid.New()
	if err := seedMemberUser(ctx, env.OwnerPool, user); err != nil {
		t.Fatalf("seeding the user: %v", err)
	}

	hash := recoveryCodeHash("recovery-code-" + uuid.NewString())
	id := uuid.New()

	err := db.WithAuthUser(ctx, env.AuthPool, user,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO totp_recovery_codes (id, user_id, code_hash) VALUES ($1, $2, $3)`,
				id, user, hash)

			return err
		})
	if err != nil {
		t.Fatalf("seeding the recovery code: %v", err)
	}

	// The first redemption marks used_at and affects exactly the one
	// not-yet-used row.
	err = db.WithAuthUser(ctx, env.AuthPool, user,
		func(ctx context.Context, tx pgx.Tx) error {
			tag, err := tx.Exec(ctx,
				`UPDATE totp_recovery_codes SET used_at = now()
				 WHERE code_hash = $1 AND used_at IS NULL`, hash)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				return fmt.Errorf("the first redemption affected %d rows, want 1",
					tag.RowsAffected())
			}

			return nil
		})
	if err != nil {
		t.Fatalf("redeeming the code for the first time: %v", err)
	}

	var usedAt *time.Time
	err = db.WithAuthUser(ctx, env.AuthPool, user,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				`SELECT used_at FROM totp_recovery_codes WHERE id = $1`, id).Scan(&usedAt)
		})
	if err != nil {
		t.Fatalf("reading back the redeemed code: %v", err)
	}
	if usedAt == nil {
		t.Fatal("used_at is still null after redemption")
	}

	// The second redemption attempt of the SAME code is rejected: the
	// predicate `used_at IS NULL` no longer matches the row, so the
	// statement succeeds but affects zero rows -- the same "qualified write,
	// zero rows" shape auth_door_test.go uses for a forbidden cross-user
	// write, here forbidding reuse instead.
	err = db.WithAuthUser(ctx, env.AuthPool, user,
		func(ctx context.Context, tx pgx.Tx) error {
			tag, err := tx.Exec(ctx,
				`UPDATE totp_recovery_codes SET used_at = now()
				 WHERE code_hash = $1 AND used_at IS NULL`, hash)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 0 {
				return fmt.Errorf("the second redemption affected %d rows, want 0 — a used "+
					"code was redeemed again", tag.RowsAffected())
			}

			return nil
		})
	if err != nil {
		t.Fatalf("attempting the second redemption: %v", err)
	}
}
