package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// setAuthScopeSQL sets the auth scope for the current transaction and nothing
// beyond it. Same shape as setScopeSQL, for the same reasons: set_config
// binds where SET LOCAL cannot, and is_local = true means the setting dies at
// COMMIT or ROLLBACK instead of riding the pooled connection to the next
// request.
//
// Every app_auth policy reads
// nullif(current_setting('app.user_id', true), '')::uuid — nullif is
// mandatory, not defensive: a reverted GUC returns the empty string, not
// NULL.
const setAuthScopeSQL = `SELECT set_config('app.user_id', $1, true)`

// ErrNoAuthUser is returned when WithAuthUser is called without a user. It is
// refused rather than defaulted, for the same reason ErrNoTenant is: a
// transaction that opens with app.user_id unset would match every
// app_auth policy against nothing, so an unscoped write would silently
// affect zero rows instead of failing loudly.
var ErrNoAuthUser = errors.New("db: a user is required to open an auth-scoped transaction")

// WithAuthUser runs fn inside a transaction scoped to ONE user. It is the
// only path that may write identity data: refresh_tokens, credential columns
// on users, and the caller's own memberships rows.
//
// The scope is set strictly between Begin and Commit, exactly like
// WithTenant, and for the same reason: a session-scoped setting would
// outlive the physical connection's release back to the pool, and the next
// unrelated request to acquire it would inherit the previous user's scope.
//
// fn receives only the transaction, never the pool, so there is no way to
// escape the scope from inside the callback.
func WithAuthUser(
	ctx context.Context,
	db Beginner,
	userID uuid.UUID,
	fn func(ctx context.Context, tx pgx.Tx) error,
) (err error) {
	if userID == uuid.Nil {
		return ErrNoAuthUser
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("db: beginning auth transaction: %w", err)
	}

	// Rollback on both failure and panic, and re-raise the panic once the
	// transaction is safely closed — swallowing it here would report success
	// for work that never ran. Rollback after a successful Commit is a
	// documented no-op in pgx, so no "did we commit" flag is needed.
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = tx.Rollback(ctx)

			panic(recovered)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	if _, err = tx.Exec(ctx, setAuthScopeSQL, userID.String()); err != nil {
		return fmt.Errorf("db: setting auth scope: %w", err)
	}

	if err = fn(ctx, tx); err != nil {
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("db: committing auth transaction: %w", err)
	}

	return nil
}

// WithAuthLookup runs fn in a READ ONLY transaction with NO user scope, for
// the one query that cannot have one: finding a user by email before anybody
// is authenticated. It sets no GUC at all — there is no user id yet to bind,
// and the narrowing for this path comes from the column grant on users
// instead of a row-level predicate (see migration 00013).
func WithAuthLookup(
	ctx context.Context,
	db TxBeginner,
	fn func(ctx context.Context, tx pgx.Tx) error,
) (err error) {
	tx, err := db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("db: beginning auth lookup transaction: %w", err)
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			_ = tx.Rollback(ctx)

			panic(recovered)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	if err = fn(ctx, tx); err != nil {
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("db: committing auth lookup transaction: %w", err)
	}

	return nil
}

// NewAuthPool opens the auth pool, connecting as app_auth. It is a third
// pool, separate from the tenant and public pools, because the two GUCs
// (app.shelter_id and app.user_id) must never be concurrent on one
// connection — see design.md P2-D1. The same cost-and-correctness contract
// as the tenant pool applies unchanged: MinConns/MinIdleConns stay at 0 so
// Neon's compute can suspend, and MaxConnIdleTime stays below
// NeonAutoSuspendAfter.
func NewAuthPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := PoolConfig(dsn)
	if err != nil {
		return nil, err
	}

	return newPool(ctx, cfg)
}
