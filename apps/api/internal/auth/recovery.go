package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gentleman/mascotapp/apps/api/internal/db/sqlcgen"
)

// Recovery codes — the way back in when the authenticator app is gone
// (data-model-core / *TOTP recovery codes provide single-use account recovery,
// non-tenant scoped*, and design P2-D8).
//
// TOTP is mandatory for `owner` and `admin` (§5.2). Mandatory second factors
// need a second door, or the first lost phone becomes a support ticket that
// ends in somebody disabling the requirement.
const (
	// RecoveryCodeCount is the ten codes a set holds, shown exactly once.
	//
	// Ten is a working supply for one account without being a list nobody
	// keeps: every unused code is a live credential to the account, so the set
	// is small on purpose.
	RecoveryCodeCount = 10

	// recoveryCodeBytes is 128 bits of entropy per code.
	//
	// Unlike a password, this is never typed from memory and never chosen by a
	// human, so it can afford full entropy — and it needs it, because the
	// hashing below is deliberately fast.
	recoveryCodeBytes = 16
)

// recoveryCodeEncoding is base32 without padding: 16 bytes become 26 characters
// a person can read off paper and type in.
//
// Base32 over base64 because the alphabet has no case distinction to lose and
// no `+`/`/` to be mangled by whatever the user pastes through. No padding
// because `=` at the end of a credential invites something to strip it.
var recoveryCodeEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// ErrRecoveryCodeInvalid is returned for a code that cannot be redeemed.
//
// One sentinel for "never issued", "belongs to somebody else" and "already
// spent". Separating them would tell an attacker whether they guessed a real
// code, and no legitimate caller behaves differently: all three are a failed
// login.
var ErrRecoveryCodeInvalid = errors.New("auth: the recovery code is not valid")

// RecoveryCode is one code as the user sees it, next to what gets stored.
//
// The two travel together because they are produced together and must never be
// paired by index at a call site: `Plaintext` is shown once and then gone
// forever, and `Hash` is the only thing that survives to recognise it.
type RecoveryCode struct {
	// Plaintext is shown to the user exactly once, at enrolment. Nothing
	// stores it — not this struct's lifetime, not a log, not the database.
	Plaintext string

	// Hash is what the row holds and what a redemption looks up by.
	Hash []byte
}

// NewRecoveryCodes draws a fresh set.
//
// Every call returns a new set: this is the enrolment path and the regeneration
// path both, and regeneration exists precisely because the old set is assumed
// compromised or lost.
func NewRecoveryCodes() ([]RecoveryCode, error) {
	codes := make([]RecoveryCode, 0, RecoveryCodeCount)

	for i := range RecoveryCodeCount {
		raw := make([]byte, recoveryCodeBytes)
		if _, err := rand.Read(raw); err != nil {
			// Returned rather than ignored. A generator that silently produced
			// a short or zero-filled code would hand the user a credential an
			// attacker can guess, and it would look exactly like a working one.
			return nil, fmt.Errorf("auth: drawing randomness for recovery code %d: %w", i, err)
		}

		plaintext := recoveryCodeEncoding.EncodeToString(raw)
		codes = append(codes, RecoveryCode{
			Plaintext: plaintext,
			Hash:      HashRecoveryCode(plaintext),
		})
	}

	return codes, nil
}

// HashRecoveryCode is how a code becomes the value stored in `code_hash`.
//
// SHA-256, NOT Argon2id — and this is a deliberate departure from `password.go`
// next door, not an oversight. A password stretcher exists to compensate for
// the low entropy of something a human chose and can remember. There is nothing
// to compensate for here: these codes are 128 uniform random bits, and no
// offline attack against them terminates. Paying Argon2id's cost would only
// slow down the one legitimate login that needs it.
//
// It is also unsalted, for the same reason `refresh_tokens.token_hash` is: the
// redemption path has to FIND the row from the submitted code alone, and a
// per-row salt would mean reading every row to test each one. A salt defends
// against precomputation over a small input space, and 2^128 is not one.
func HashRecoveryCode(plaintext string) []byte {
	sum := sha256.Sum256([]byte(plaintext))

	return sum[:]
}

// RegenerateRecoveryCodes replaces the user's whole set.
//
// It takes the transaction, never a pool. `SET LOCAL app.user_id` and the
// transaction boundary belong to `db.WithAuthUser`, and the delete and the ten
// inserts have to be atomic: a failure between them leaves an account with
// fewer codes than it should have, or none.
func RegenerateRecoveryCodes(
	ctx context.Context,
	tx pgx.Tx,
	userID uuid.UUID,
	codes []RecoveryCode,
) error {
	// An empty set clears the user's last way back into their account and looks
	// like success — the database cannot object, because deleting ten rows and
	// inserting none is a legal transaction. This guard is the only thing that
	// refuses it, and mutation proved it load-bearing: remove it and the caller
	// commits an account holding zero recovery codes.
	//
	// Its POSITION is a different claim, and a weaker one than an earlier
	// version of this comment made. Moving it below the delete changes nothing
	// a caller can observe, because `db.WithAuthUser` rolls the transaction
	// back on any error the callback returns — the rollback, not this ordering,
	// is what keeps the old set alive. It stays first because issuing a
	// statement you already know you will undo is work for nothing.
	if len(codes) == 0 {
		return errors.New("auth: refusing to regenerate recovery codes with an empty set; " +
			"this would delete the user's codes and leave them none")
	}

	queries := sqlcgen.New(tx)

	if _, err := queries.DeleteRecoveryCodesForUser(ctx, userID); err != nil {
		return fmt.Errorf("auth: clearing the previous recovery codes: %w", err)
	}

	for i, code := range codes {
		// v7, not v4: the id is time-ordered, so a set written together lands
		// together in the primary key's index instead of scattering across it.
		id, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("auth: generating an id for recovery code %d: %w", i, err)
		}

		err = queries.InsertRecoveryCode(ctx, sqlcgen.InsertRecoveryCodeParams{
			ID:       id,
			UserID:   userID,
			CodeHash: code.Hash,
		})
		if err != nil {
			return fmt.Errorf("auth: storing recovery code %d: %w", i, err)
		}
	}

	return nil
}

// RedeemRecoveryCode spends one code, returning ErrRecoveryCodeInvalid if it
// cannot be spent.
//
// The statement carries no `user_id`, on purpose: the transaction is already
// scoped by `db.WithAuthUser`, and the RLS policy `user_id = app.user_id` is
// what keeps one account's codes out of another's reach. Passing a user id in
// here would create a second, weaker answer to a question the database is
// already answering — and a caller that took that id from the request would
// hand the check to the attacker.
func RedeemRecoveryCode(ctx context.Context, tx pgx.Tx, plaintext string) error {
	affected, err := sqlcgen.New(tx).RedeemRecoveryCode(ctx, HashRecoveryCode(plaintext))
	if err != nil {
		return fmt.Errorf("auth: redeeming a recovery code: %w", err)
	}

	// THE line this function exists for. The query's `used_at IS NULL` makes a
	// spent code match nothing, and RLS makes another user's code match
	// nothing; in both cases the statement SUCCEEDS and touches no row. Without
	// this, a used code would complete a login and the database would have done
	// everything right.
	if affected == 0 {
		return ErrRecoveryCodeInvalid
	}

	return nil
}
