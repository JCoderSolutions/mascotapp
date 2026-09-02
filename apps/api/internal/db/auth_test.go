package db_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gentleman/mascotapp/apps/api/internal/db"
)

// ---------------------------------------------------------------------------
// WithAuthUser
//
// Stubs reused from tenant_test.go (recordingTx, recordingPool, newPool):
// WithAuthUser needs exactly the Beginner shape WithTenant needs, so the same
// recording stub proves the same class of guarantee without a database.
// ---------------------------------------------------------------------------

func TestWithAuthUser_RejectsNilUserBeforeOpeningATransaction(t *testing.T) {
	t.Parallel()

	pool := newPool()
	called := false

	err := db.WithAuthUser(context.Background(), pool, uuid.Nil,
		func(context.Context, pgx.Tx) error {
			called = true

			return nil
		})

	if !errors.Is(err, db.ErrNoAuthUser) {
		t.Fatalf("expected ErrNoAuthUser, got %v", err)
	}
	// The check must happen BEFORE Begin. An unscoped write would match a
	// policy that is false, silently affecting zero rows instead of failing.
	if pool.begins != 0 {
		t.Fatalf("opened %d transactions for a nil user; want 0", pool.begins)
	}
	if called {
		t.Fatal("ran the callback for a nil user")
	}
}

func TestWithAuthUser_SetsScopeThenRunsThenCommits(t *testing.T) {
	t.Parallel()

	pool := newPool()
	user := uuid.MustParse("33333333-3333-4333-8333-333333333333")

	var gotTx pgx.Tx
	err := db.WithAuthUser(context.Background(), pool, user,
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

// This is the security assertion of the file, the same shape as
// TestWithTenant_PassesTheTenantAsABindParameterNotAsSQLText. set_config binds;
// SET LOCAL cannot, which would force the user id into SQL text.
func TestWithAuthUser_PassesTheUserIDAsABindParameterNotAsSQLText(t *testing.T) {
	t.Parallel()

	pool := newPool()
	user := uuid.MustParse("44444444-4444-4444-8444-444444444444")

	err := db.WithAuthUser(context.Background(), pool, user,
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
	if !strings.Contains(sql, "app.user_id") {
		t.Fatalf("did not set app.user_id: %q", sql)
	}
	if strings.Contains(sql, user.String()) {
		t.Fatalf("interpolated the user id into SQL text instead of binding it: %q", sql)
	}
	if !strings.Contains(sql, "$1") {
		t.Fatalf("expected a bind placeholder, got %q", sql)
	}

	args := pool.tx.execArgs[0]
	if len(args) != 1 {
		t.Fatalf("passed %d arguments, want exactly the user id", len(args))
	}
	if got, ok := args[0].(string); !ok || got != user.String() {
		t.Fatalf("bound argument = %#v, want the user id as a string", args[0])
	}

	// is_local = true: the setting must die with the transaction, or it rides
	// the pooled physical connection to whichever request acquires it next.
	if !strings.Contains(sql, "true") {
		t.Fatalf("set_config is not transaction-local: %q", sql)
	}
}

func TestWithAuthUser_RollsBackWhenTheCallbackFails(t *testing.T) {
	t.Parallel()

	pool := newPool()
	sentinel := errors.New("boom")

	err := db.WithAuthUser(context.Background(), pool, uuid.New(),
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

func TestWithAuthUser_RollsBackAndRePanicsWhenTheCallbackPanics(t *testing.T) {
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

		_ = db.WithAuthUser(context.Background(), pool, uuid.New(),
			func(context.Context, pgx.Tx) error { panic("kaboom") })
	}()

	if pool.tx.rollbacks != 1 {
		t.Fatalf("rollbacks = %d after a panic, want 1", pool.tx.rollbacks)
	}
	if pool.tx.commits != 0 {
		t.Fatalf("committed %d times after a panic; want 0", pool.tx.commits)
	}
}

// ---------------------------------------------------------------------------
// WithAuthLookup
//
// Stub reused from db_test.go (recordingTxPool, newTxPool): WithAuthLookup
// needs exactly the TxBeginner shape WithPublic needs.
// ---------------------------------------------------------------------------

func TestWithAuthLookup_OpensReadOnlyAndSetsNoGUC(t *testing.T) {
	t.Parallel()

	pool := newTxPool()

	err := db.WithAuthLookup(context.Background(), pool,
		func(context.Context, pgx.Tx) error { return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pool.opts) != 1 {
		t.Fatalf("opened %d transactions, want 1", len(pool.opts))
	}
	if pool.opts[0].AccessMode != pgx.ReadOnly {
		t.Errorf("AccessMode = %v, want ReadOnly", pool.opts[0].AccessMode)
	}

	// WithAuthLookup exists for the one query that runs before anybody is
	// authenticated: finding a user by email. There is no user id yet to bind,
	// so it sets no GUC at all — unlike WithAuthUser and WithTenant.
	if len(pool.tx.execSQL) != 0 {
		t.Fatalf("executed %d statements before the callback; want none — "+
			"WithAuthLookup sets no GUC", len(pool.tx.execSQL))
	}
	if pool.tx.commits != 1 {
		t.Errorf("commits = %d, want 1", pool.tx.commits)
	}
}

func TestWithAuthLookup_RollsBackWhenTheCallbackFails(t *testing.T) {
	t.Parallel()

	pool := newTxPool()
	sentinel := errors.New("boom")

	err := db.WithAuthLookup(context.Background(), pool,
		func(context.Context, pgx.Tx) error { return sentinel })

	if !errors.Is(err, sentinel) {
		t.Fatalf("expected the callback's error, got %v", err)
	}
	if pool.tx.rollbacks != 1 {
		t.Errorf("rollbacks = %d, want 1", pool.tx.rollbacks)
	}
	if pool.tx.commits != 0 {
		t.Errorf("commits = %d, want 0", pool.tx.commits)
	}
}

func TestWithAuthLookup_RollsBackAndRePanics(t *testing.T) {
	t.Parallel()

	pool := newTxPool()

	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic was swallowed; it must propagate after the rollback")
			}
		}()

		_ = db.WithAuthLookup(context.Background(), pool,
			func(context.Context, pgx.Tx) error { panic("kaboom") })
	}()

	if pool.tx.rollbacks != 1 {
		t.Errorf("rollbacks = %d after a panic, want 1", pool.tx.rollbacks)
	}
}

