package rlstest_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gentleman/mascotapp/apps/api/internal/db"
	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
	"github.com/gentleman/mascotapp/apps/api/internal/db/rlstest"
)

// The two properties every policy in this schema rests on, asserted against a
// real pooled connection rather than against WithTenant's unit-test stub:
// the scope does not outlive its transaction, and access that nobody granted is
// refused rather than merely filtered.

// sqlstateInsufficientPrivilege is what PostgreSQL raises for a missing grant.
// Named because the number alone says nothing at a call site.
const sqlstateInsufficientPrivilege = "42501"

// errProbeFailed aborts a WithTenant call to exercise the rollback path.
var errProbeFailed = errors.New("rlstest: failing on purpose to force a rollback")

// readScope returns app.shelter_id as seen on this connection, outside any
// transaction, along with whether it is set at all.
func readScope(t *testing.T, conn *pgxpool.Conn) (string, bool) {
	t.Helper()

	var scope *string
	err := conn.QueryRow(context.Background(),
		`SELECT current_setting('app.shelter_id', true)`).Scan(&scope)
	if err != nil {
		t.Fatalf("reading app.shelter_id: %v", err)
	}
	if scope == nil || *scope == "" {
		return "", false
	}

	return *scope, true
}

// The scope is set with `set_config(..., true)` — transaction-local — so
// PostgreSQL reverts it at COMMIT and at ROLLBACK as part of transaction
// teardown. There is no cleanup path to forget.
//
// The commit and rollback halves are separate cases because a leak on only one
// of them is the realistic bug: the cleanup that runs on the happy path and is
// skipped by an early return.
//
// The connection is ACQUIRED and held, so the read afterwards is the same
// physical connection the transaction ran on. Reading through the pool instead
// would prove nothing — a different connection never had the scope to begin
// with, and the test would pass with the scope pinned to the session.
func TestTenantScope_DoesNotOutliveItsTransaction(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		fn   func(ctx context.Context, tx pgx.Tx) error
		// wantErr is what WithTenant must return, so a case cannot pass by
		// taking a path it was not meant to take.
		wantErr error
	}{
		{
			name: "after the transaction commits",
			fn:   func(context.Context, pgx.Tx) error { return nil },
		},
		{
			name:    "after the transaction rolls back",
			fn:      func(context.Context, pgx.Tx) error { return errProbeFailed },
			wantErr: errProbeFailed,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn, err := env.TenantPool.Acquire(ctx)
			if err != nil {
				t.Fatalf("acquiring a connection: %v", err)
			}
			defer conn.Release()

			if _, set := readScope(t, conn); set {
				t.Fatal("the connection already carried a scope before the transaction, " +
					"so this case could not tell a leak from the starting state")
			}

			// conn, not the pool: WithTenant takes a Beginner, and holding the
			// connection is what makes the read below the same session.
			err = db.WithTenant(ctx, conn, env.ShelterA,
				func(ctx context.Context, tx pgx.Tx) error {
					// Proves the scope was actually set. Without this the case
					// would pass just as well if WithTenant never set anything.
					var inside string
					if err := tx.QueryRow(ctx,
						`SELECT current_setting('app.shelter_id', true)`).Scan(&inside); err != nil {
						return err
					}
					if inside != env.ShelterA.String() {
						t.Errorf("inside the transaction the scope is %q, want %q",
							inside, env.ShelterA)
					}

					return tc.fn(ctx, tx)
				})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("WithTenant returned %v, want %v", err, tc.wantErr)
			}

			if scope, set := readScope(t, conn); set {
				t.Errorf("the connection still carries scope %q after the transaction "+
					"ended. pgxpool hands this connection back for reuse, so the next "+
					"unrelated request would inherit another tenant's scope and every "+
					"policy would evaluate correctly against the wrong shelter", scope)
			}
		})
	}
}

// The case above holds the connection. This one releases it and takes it back,
// which is the shape the production failure actually has: request 1 finishes,
// the connection returns to the pool, request 2 picks it up.
//
// MaxConns is 1 so "the same physical connection" is a certainty rather than a
// coincidence. With the default pool size this test would pass by luck most of
// the time, which is worse than not having it.
func TestTenantScope_DoesNotFollowAConnectionBackIntoThePool(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	config := env.TenantPool.Config().Copy()
	config.MaxConns = 1
	config.MinConns = 1

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("building a single-connection pool: %v", err)
	}
	defer pool.Close()

	first, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquiring: %v", err)
	}
	firstPID := first.Conn().PgConn().PID()

	if err := db.WithTenant(ctx, first, env.ShelterA,
		func(context.Context, pgx.Tx) error { return nil }); err != nil {
		t.Fatalf("running a scoped transaction: %v", err)
	}
	first.Release()

	second, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("re-acquiring: %v", err)
	}
	defer second.Release()

	if pid := second.Conn().PgConn().PID(); pid != firstPID {
		t.Fatalf("the pool handed back backend %d instead of %d, so this proved nothing "+
			"about connection reuse", pid, firstPID)
	}

	if scope, set := readScope(t, second); set {
		t.Errorf("a connection came back out of the pool still scoped to %q. This is the "+
			"cross-tenant read WithTenant's transaction-local set_config exists to make "+
			"impossible, and it raises no error anywhere", scope)
	}
}

// Fail closed: a query that forgets WithTenant returns nothing, rather than
// everything or somebody else's rows.
//
// The two connection states are separate cases, and that separation is the whole
// point — it is what this test was for.
//
// A GUC that was NEVER set yields NULL under missing_ok. A GUC that was set
// inside a transaction and reverted at COMMIT or ROLLBACK does NOT go back to
// unset: it goes back to the EMPTY STRING. Which means the fail-closed contract
// held on a virgin connection and raised 22P02 on a reused one — and after the
// first request, every pooled connection is a reused one.
//
// The first run of this test failed on exactly that, against the migration as
// T-01-013 wrote it. The fix is `nullif(current_setting(...), ”)` in every
// policy; this case is what keeps it there.
func TestUnscopedQuery_SeesZeroRowsRatherThanTheWrongRows(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	if err := seedShelters(ctx, env.OwnerPool, env.ShelterA, env.ShelterB); err != nil {
		t.Fatalf("seeding shelters: %v", err)
	}

	// Every tenant table, derived from the catalog rather than named here.
	//
	// Mutation on 2026-08-30 is why: with `shelters` alone, removing the nullif
	// from memberships' policy survived. The property has to hold for every
	// policy, and a hand-written list is the next thing to fall out of step —
	// this way a migration that adds a tenant table brings its case with it.
	tables := []string{}
	for _, table := range rlstest.Schema.Tenant {
		if _, pending := rlstest.Schema.Pending[table]; !pending {
			tables = append(tables, table)
		}
	}
	if len(tables) == 0 {
		t.Fatal("no tenant table has landed yet, so this proved nothing")
	}

	// A row per table, or the policy expression is never evaluated and the
	// assertion is vacuous. The owner-side count below is what proves it.
	memberUser := uuid.New()
	if err := seedMemberUser(ctx, env.OwnerPool, memberUser); err != nil {
		t.Fatalf("seeding a user: %v", err)
	}
	if err := seedMembership(ctx, env.OwnerPool, memberUser, env.ShelterA); err != nil {
		t.Fatalf("seeding a membership: %v", err)
	}

	// A FRESH single-connection pool per table, built inside the loop.
	//
	// MaxConns 1 makes "the same physical connection" a certainty rather than a
	// coincidence of pool scheduling. Building it per table is what makes the
	// virgin case actually virgin: with one shared pool, only the FIRST table
	// got an untouched connection, and mutation caught exactly that — dropping
	// missing_ok survived, because by the time shelters ran, the connection had
	// already been through memberships.
	freshPool := func(t *testing.T) *pgxpool.Pool {
		t.Helper()

		config := env.TenantPool.Config().Copy()
		config.MaxConns = 1
		config.MinConns = 1

		pool, err := pgxpool.NewWithConfig(ctx, config)
		if err != nil {
			t.Fatalf("building a single-connection pool: %v", err)
		}
		t.Cleanup(pool.Close)

		return pool
	}

	unscopedCount := func(t *testing.T, pool *pgxpool.Pool, table string) int {
		t.Helper()

		var n int
		err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&n)
		if err != nil {
			t.Fatalf("an unscoped read of %s raised an error instead of returning zero "+
				"rows. The spec requires it to fail CLOSED — zero rows — not loudly, and "+
				"a contract that depends on whether this connection served an earlier "+
				"request is not a contract: %v", table, err)
		}

		return n
	}

	exercised := 0
	for _, table := range tables {
		var rows int
		if err := env.OwnerPool.QueryRow(ctx,
			`SELECT count(*) FROM `+table).Scan(&rows); err != nil {
			t.Fatalf("counting %s as the owner: %v", table, err)
		}
		if rows == 0 {
			t.Errorf("%s holds no rows at all, so its policy expression is never "+
				"evaluated and its case below proves nothing. Seed it here", table)

			continue
		}
		exercised++

		pool := freshPool(t)

		t.Run(table+" on a connection that never carried a scope", func(t *testing.T) {
			if n := unscopedCount(t, pool, table); n != 0 {
				t.Errorf("a query issued outside WithTenant saw %d of %d rows. Every "+
					"policy in this schema reads app.shelter_id, so this crosses every "+
					"tenant at once", n, rows)
			}
		})

		// Puts this connection into the reused state, which is where the empty
		// string — not NULL — is what current_setting returns.
		var scoped int
		if err := db.WithTenant(ctx, pool, env.ShelterA,
			func(ctx context.Context, tx pgx.Tx) error {
				return tx.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&scoped)
			}); err != nil {
			t.Fatalf("reading %s under scope: %v", table, err)
		}

		t.Run(table+" on a connection that has already carried one", func(t *testing.T) {
			if n := unscopedCount(t, pool, table); n != 0 {
				t.Errorf("a query issued outside WithTenant saw %d rows of %s on a reused "+
					"connection (a scoped read sees %d)", n, table, scoped)
			}
		})
	}

	t.Logf("fail-closed verified for %d of %d landed tenant tables", exercised, len(tables))
}

// D9's default-deny, proven against a table that has everything EXCEPT a grant.
//
// It matters that the refusal is 42501 and not an empty result. A missing grant
// that merely filtered would be indistinguishable from correct isolation, and
// the day somebody forgot a GRANT the suite would stay green while the feature
// silently returned nothing.
func TestATableWithNoGrant_IsUnreachableEvenUnderAValidScope(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	dbtest.MustExec(t, env.OwnerPool, `
		CREATE TABLE ungranted_probe (
			id         uuid NOT NULL PRIMARY KEY,
			shelter_id uuid NOT NULL
		)`)
	t.Cleanup(func() {
		_, _ = env.OwnerPool.Exec(ctx, `DROP TABLE IF EXISTS ungranted_probe`)
	})
	dbtest.MustExec(t, env.OwnerPool, `ALTER TABLE ungranted_probe ENABLE ROW LEVEL SECURITY`)
	dbtest.MustExec(t, env.OwnerPool, `ALTER TABLE ungranted_probe FORCE ROW LEVEL SECURITY`)
	dbtest.MustExec(t, env.OwnerPool, `CREATE POLICY tenant_isolation ON ungranted_probe
		FOR ALL TO app_tenant
		USING      (shelter_id = current_setting('app.shelter_id', true)::uuid)
		WITH CHECK (shelter_id = current_setting('app.shelter_id', true)::uuid)`)
	// And deliberately no GRANT.

	err := db.WithTenant(ctx, env.TenantPool, env.ShelterA,
		func(ctx context.Context, tx pgx.Tx) error {
			var n int

			return tx.QueryRow(ctx, `SELECT count(*) FROM ungranted_probe`).Scan(&n)
		})

	requireInsufficientPrivilege(t, err,
		"a table with RLS and a correct policy but no grant was readable by app_tenant. "+
			"D9's default-deny is what makes forgetting a GRANT loud instead of silent")
}

// refresh_tokens is default-deny for this phase: RLS on, no policy, no grant.
// Both halves are asserted, because either one alone would be enough to refuse
// a read and neither alone is the contract.
func TestRefreshTokens_RefuseAppTenantEntirely(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		sql  string
		args []any
	}{
		{"reading", `SELECT count(*) FROM refresh_tokens`, nil},
		{
			"writing",
			`INSERT INTO refresh_tokens (id, user_id, token_hash, family_id, expires_at)
			 VALUES ($1, $2, $3, $4, now())`,
			[]any{uuid.New(), uuid.New(), []byte("hash"), uuid.New()},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Under a VALID scope, so a refusal cannot be the scope's doing.
			err := db.WithTenant(ctx, env.TenantPool, env.ShelterA,
				func(ctx context.Context, tx pgx.Tx) error {
					_, err := tx.Exec(ctx, tc.sql, tc.args...)

					return err
				})

			requireInsufficientPrivilege(t, err,
				"app_tenant reached refresh_tokens. This phase gives it no policy and no "+
					"grant on purpose: the right scope for a refresh token is the USER, "+
					"and no app.user_id exists yet, so denying everyone is stricter than "+
					"any policy Phase 01 could write")
		})
	}
}

func requireInsufficientPrivilege(t *testing.T, err error, why string) {
	t.Helper()

	if err == nil {
		t.Fatal(why)
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != sqlstateInsufficientPrivilege {
		t.Fatalf("the statement failed, but not with %s, so what refused it is unproven: %v",
			sqlstateInsufficientPrivilege, err)
	}
}
