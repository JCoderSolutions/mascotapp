// Package db owns every path from Go into PostgreSQL: the pools, the embedded
// migrations, and the transaction wrappers that carry tenant scope.
package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// setScopeSQL sets the tenant scope for the current transaction and nothing
// beyond it.
//
// Two details here are load-bearing, and both are ADR-0002 requirements:
//
//   - It is `set_config`, never `SET LOCAL`. `SET LOCAL` is a utility statement
//     and cannot take bind parameters, so using it would force the tenant id to
//     be pasted into SQL text. Today that id comes from a validated JWT claim,
//     so there is no injection; the ban exists so the unsafe shape is not the
//     one that gets copied forward the day the source is less trustworthy.
//   - The third argument is `true`, meaning transaction-local. PostgreSQL
//     reverts it at COMMIT or ROLLBACK as part of transaction teardown, so there
//     is no cleanup path left to forget or to skip on an early return.
const setScopeSQL = `SELECT set_config('app.shelter_id', $1, true)`

// ErrNoTenant is returned when WithTenant is called without a tenant. It is
// refused rather than defaulted: a transaction that opens with app.shelter_id
// unset makes every policy match nothing, so the caller would see an empty
// result instead of an error and read it as "no data".
var ErrNoTenant = errors.New("db: a tenant is required to open a scoped transaction")

// Beginner is the slice of *pgxpool.Pool that WithTenant needs.
//
// Depending on this rather than on the concrete pool is what lets the wrapper's
// contract — set scope, run, commit, roll back on failure and on panic — be
// asserted without a database. The isolation those rules protect is the whole
// point of this phase, and a guarantee only exercised by integration tests is
// one that goes unexercised whenever the Docker daemon is down.
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// TxBeginner is the same idea for WithPublic, which needs the options-taking
// form so it can open the transaction read-only.
type TxBeginner interface {
	BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error)
}

// WithTenant runs fn inside a transaction whose tenant scope is set for the
// lifetime of that transaction and no longer. It is the only sanctioned path to
// tenant data: every RLS policy in this schema reads app.shelter_id, so a query
// issued outside this wrapper sees zero rows rather than the wrong rows.
//
// The scope is set strictly between Begin and Commit, and never on the pool.
// pgxpool hands out physical connections and takes them back for reuse, so a
// session-scoped setting would survive the release and the next unrelated
// request to acquire that connection would inherit the previous tenant's scope.
// Every policy would then evaluate correctly against the wrong shelter: a silent
// cross-tenant read, with no error raised anywhere. For the same reason the GUC
// is deliberately not set in AfterConnect or BeforeAcquire.
//
// fn receives only the transaction. The pool is never handed out, so there is no
// way to escape the scope from inside the callback.
func WithTenant(
	ctx context.Context,
	db Beginner,
	shelterID uuid.UUID,
	fn func(ctx context.Context, tx pgx.Tx) error,
) (err error) {
	if shelterID == uuid.Nil {
		return ErrNoTenant
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("db: beginning tenant transaction: %w", err)
	}

	// Rollback on both failure and panic. Rollback after a successful Commit is
	// a documented no-op in pgx, so this needs no "did we commit" flag; the
	// panic is re-raised once the transaction is safely closed, because
	// swallowing it here would return a nil error for work that never ran.
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = tx.Rollback(ctx)

			panic(recovered)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	if _, err = tx.Exec(ctx, setScopeSQL, shelterID.String()); err != nil {
		return fmt.Errorf("db: setting tenant scope: %w", err)
	}

	if err = fn(ctx, tx); err != nil {
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("db: committing tenant transaction: %w", err)
	}

	return nil
}

// WithPublic runs fn inside a read-only transaction on the public catalog pool.
//
// It sets no tenant scope, and that is the point: the public catalog is not
// scoped to a shelter, it is filtered by the public_catalog policy attached to
// the app_public role. Setting app.shelter_id here would quietly narrow the
// public site to whichever shelter happened to be in context.
//
// READ ONLY is defence in depth, not the control. The real authority is that
// app_public holds SELECT grants and nothing else; this only makes a write fail
// at the transaction rather than at the grant, one layer earlier and with a
// clearer error.
func WithPublic(
	ctx context.Context,
	db TxBeginner,
	fn func(ctx context.Context, tx pgx.Tx) error,
) (err error) {
	tx, err := db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("db: beginning public transaction: %w", err)
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
		return fmt.Errorf("db: committing public transaction: %w", err)
	}

	return nil
}
