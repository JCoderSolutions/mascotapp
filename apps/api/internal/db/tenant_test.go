package db_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/gentleman/mascotapp/apps/api/internal/db"
)

// ---------------------------------------------------------------------------
// Stubs
//
// recordingTx embeds pgx.Tx as a nil interface on purpose. Only the methods
// WithTenant is allowed to touch are overridden; anything else panics with a
// nil-pointer dereference. That turns "WithTenant called something unexpected"
// from an invisible behaviour into a loud test failure.
// ---------------------------------------------------------------------------

type recordingTx struct {
	pgx.Tx

	execSQL  []string
	execArgs [][]any
	execErr  error

	commits   int
	rollbacks int
	commitErr error

	// order records Commit/Rollback as they happen, so a test can prove a
	// rollback did not follow a successful commit.
	order []string
}

func (tx *recordingTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	tx.execSQL = append(tx.execSQL, sql)
	tx.execArgs = append(tx.execArgs, args)

	return pgconn.CommandTag{}, tx.execErr
}

func (tx *recordingTx) Commit(context.Context) error {
	tx.commits++
	tx.order = append(tx.order, "commit")

	return tx.commitErr
}

func (tx *recordingTx) Rollback(context.Context) error {
	tx.rollbacks++
	tx.order = append(tx.order, "rollback")

	return nil
}

type recordingPool struct {
	tx       *recordingTx
	begins   int
	beginErr error
}

func (p *recordingPool) Begin(context.Context) (pgx.Tx, error) {
	p.begins++
	if p.beginErr != nil {
		return nil, p.beginErr
	}

	return p.tx, nil
}

func newPool() *recordingPool {
	return &recordingPool{tx: &recordingTx{}}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestWithTenant_RejectsNilTenantBeforeOpeningATransaction(t *testing.T) {
	t.Parallel()

	pool := newPool()
	called := false

	err := db.WithTenant(context.Background(), pool, uuid.Nil,
		func(context.Context, pgx.Tx) error {
			called = true

			return nil
		})

	if !errors.Is(err, db.ErrNoTenant) {
		t.Fatalf("expected ErrNoTenant, got %v", err)
	}
	// The check must happen BEFORE Begin. An empty tenant that opens a
	// transaction leaves app.shelter_id unset on a live connection, and every
	// policy then evaluates against nothing.
	if pool.begins != 0 {
		t.Fatalf("opened %d transactions for a nil tenant; want 0", pool.begins)
	}
	if called {
		t.Fatal("ran the callback for a nil tenant")
	}
}

func TestWithTenant_SetsScopeThenRunsThenCommits(t *testing.T) {
	t.Parallel()

	pool := newPool()
	shelter := uuid.MustParse("11111111-1111-4111-8111-111111111111")

	var gotTx pgx.Tx
	err := db.WithTenant(context.Background(), pool, shelter,
		func(_ context.Context, tx pgx.Tx) error {
			// The scope must already be set by the time the callback runs.
			if len(pool.tx.execSQL) != 1 {
				t.Fatalf("callback ran after %d statements; want the scope set first",
					len(pool.tx.execSQL))
			}
			gotTx = tx

			return nil
		})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotTx != pgx.Tx(pool.tx) {
		t.Fatal("callback did not receive the transaction the pool handed out")
	}
	if pool.tx.commits != 1 {
		t.Fatalf("commits = %d, want 1", pool.tx.commits)
	}
	if pool.tx.rollbacks != 0 {
		t.Fatalf("rollbacks = %d after a clean run, want 0", pool.tx.rollbacks)
	}
}

// This is the security assertion of the file. ADR-0002 forbids SET LOCAL
// precisely because it is a utility statement that cannot take bind parameters,
// which forces the tenant id to be pasted into SQL text. If the id ever appears
// inside the statement rather than in the argument list, that ban has been
// silently undone.
func TestWithTenant_PassesTheTenantAsABindParameterNotAsSQLText(t *testing.T) {
	t.Parallel()

	pool := newPool()
	shelter := uuid.MustParse("22222222-2222-4222-8222-222222222222")

	err := db.WithTenant(context.Background(), pool, shelter,
		func(context.Context, pgx.Tx) error { return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pool.tx.execSQL) != 1 {
		t.Fatalf("ran %d statements before the callback; want exactly 1", len(pool.tx.execSQL))
	}
	sql := pool.tx.execSQL[0]

	if strings.Contains(strings.ToUpper(sql), "SET LOCAL") {
		t.Fatalf("used SET LOCAL, which ADR-0002 forbids: %q", sql)
	}
	if !strings.Contains(sql, "set_config") {
		t.Fatalf("expected set_config, got %q", sql)
	}
	if !strings.Contains(sql, "app.shelter_id") {
		t.Fatalf("did not set app.shelter_id: %q", sql)
	}
	if strings.Contains(sql, shelter.String()) {
		t.Fatalf("interpolated the tenant id into SQL text instead of binding it: %q", sql)
	}
	if !strings.Contains(sql, "$1") {
		t.Fatalf("expected a bind placeholder, got %q", sql)
	}

	args := pool.tx.execArgs[0]
	if len(args) != 1 {
		t.Fatalf("passed %d arguments, want exactly the tenant id", len(args))
	}
	// set_config's second parameter is text, so the id travels as a string.
	if got, ok := args[0].(string); !ok || got != shelter.String() {
		t.Fatalf("bound argument = %#v, want the tenant id as a string", args[0])
	}

	// The third argument of set_config is is_local. If it were false the
	// setting would outlive the transaction and leak to the next request that
	// acquires this physical connection from the pool.
	if !strings.Contains(sql, "true") {
		t.Fatalf("set_config is not transaction-local: %q", sql)
	}
}

func TestWithTenant_RollsBackWhenTheCallbackFails(t *testing.T) {
	t.Parallel()

	pool := newPool()
	sentinel := errors.New("boom")

	err := db.WithTenant(context.Background(), pool, uuid.New(),
		func(context.Context, pgx.Tx) error { return sentinel })

	if !errors.Is(err, sentinel) {
		t.Fatalf("expected the callback's error to survive, got %v", err)
	}
	if pool.tx.commits != 0 {
		t.Fatalf("committed %d times after a failure; want 0", pool.tx.commits)
	}
	if pool.tx.rollbacks != 1 {
		t.Fatalf("rollbacks = %d, want 1", pool.tx.rollbacks)
	}
}

func TestWithTenant_RollsBackAndRePanicsWhenTheCallbackPanics(t *testing.T) {
	t.Parallel()

	pool := newPool()

	func() {
		defer func() {
			recovered := recover()
			if recovered == nil {
				t.Error("panic was swallowed; it must propagate after the rollback")

				return
			}
			if got, ok := recovered.(string); !ok || got != "kaboom" {
				t.Errorf("re-panicked with %#v, want the original value", recovered)
			}
		}()

		_ = db.WithTenant(context.Background(), pool, uuid.New(),
			func(context.Context, pgx.Tx) error { panic("kaboom") })
	}()

	// A panic must not leave the transaction open. Without this the physical
	// connection goes back to the pool still inside a transaction.
	if pool.tx.rollbacks != 1 {
		t.Fatalf("rollbacks = %d after a panic, want 1", pool.tx.rollbacks)
	}
	if pool.tx.commits != 0 {
		t.Fatalf("committed %d times after a panic; want 0", pool.tx.commits)
	}
}

func TestWithTenant_DoesNotRunTheCallbackWhenBeginFails(t *testing.T) {
	t.Parallel()

	pool := newPool()
	pool.beginErr = errors.New("pool exhausted")
	called := false

	err := db.WithTenant(context.Background(), pool, uuid.New(),
		func(context.Context, pgx.Tx) error {
			called = true

			return nil
		})

	if err == nil {
		t.Fatal("expected an error when Begin fails")
	}
	if called {
		t.Fatal("ran the callback without a transaction")
	}
}

// If the scope statement fails, the callback must never run: it would execute
// against a transaction with no tenant set, where every policy matches nothing
// and the caller sees an empty result rather than an error.
func TestWithTenant_DoesNotRunTheCallbackWhenTheScopeCannotBeSet(t *testing.T) {
	t.Parallel()

	pool := newPool()
	pool.tx.execErr = errors.New("cannot set scope")
	called := false

	err := db.WithTenant(context.Background(), pool, uuid.New(),
		func(context.Context, pgx.Tx) error {
			called = true

			return nil
		})

	if err == nil {
		t.Fatal("expected an error when the scope cannot be set")
	}
	if called {
		t.Fatal("ran the callback with no tenant scope set")
	}
	if pool.tx.rollbacks != 1 {
		t.Fatalf("rollbacks = %d, want 1", pool.tx.rollbacks)
	}
	if pool.tx.commits != 0 {
		t.Fatalf("committed %d times; want 0", pool.tx.commits)
	}
}

func TestWithTenant_ReturnsCommitFailures(t *testing.T) {
	t.Parallel()

	pool := newPool()
	pool.tx.commitErr = errors.New("commit refused")

	err := db.WithTenant(context.Background(), pool, uuid.New(),
		func(context.Context, pgx.Tx) error { return nil })

	if err == nil {
		t.Fatal("a failed commit was reported as success")
	}
	if !errors.Is(err, pool.tx.commitErr) {
		t.Fatalf("expected the commit error to survive, got %v", err)
	}
}
