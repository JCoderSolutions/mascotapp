package rlstest_test

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
)

// The PostgreSQL column-privilege semantics that `00015_column_grants` rests on,
// pinned against a scratch table and a scratch role that this file creates and
// drops itself.
//
// It is deliberately self-contained. Asserting these four facts against
// `shelters` would prove that `00015` behaves as written; asserting them against
// a table nobody else touches proves the DATABASE behaves as P2-D4 assumes,
// which is the thing the design actually rests on. If PostgreSQL ever changed
// here, this file goes red on its own instead of a shelter test going red for a
// reason nobody could place.
//
// The four facts, verified live on PG 17 on 2026-09-02 and pinned here so that a
// future grant change cannot break registration in silence:
//
//  1. An INSERT that OMITS an ungranted column succeeds, and the column takes
//     its DEFAULT. This is the whole reason shelter registration still works
//     without holding a privilege on `status`.
//  2. An INSERT that NAMES that column raises 42501.
//  3. An UPDATE of that column raises the same 42501.
//  4. A granted column still writes — without this the other three would also
//     pass against a role that simply cannot do anything at all.
//
// THE GOTCHA, and it is load-bearing: the refusal reads
// `permission denied for TABLE <t>` — it says TABLE, not column. A test matching
// the word "column", or any substring of that message, passes for the wrong
// reason or fails for no reason. Every assertion below is on the SQLSTATE and
// nothing else.
func TestColumnPrivilege_SemanticsSliceBDependsOn(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	table, probe := newColumnPrivilegeScratch(t, env)

	// The row every case works against. It is inserted through the probe role,
	// not the owner, because case 1 IS the assertion that this insert works.
	id := uuid.New()

	t.Run("insert_omitting_an_ungranted_column_succeeds_and_takes_the_default", func(t *testing.T) {
		_, err := probe.Exec(ctx,
			fmt.Sprintf(`INSERT INTO %s (id, label) VALUES ($1, $2)`, table), id, "first")
		if err != nil {
			t.Fatalf("inserting without naming the ungranted column: %v. This is the "+
				"privilege shelter registration relies on -- if it fails, registration "+
				"cannot create a row it is not allowed to set `status` on", err)
		}

		var state string
		if err := probe.QueryRow(ctx,
			fmt.Sprintf(`SELECT state FROM %s WHERE id = $1`, table), id).Scan(&state); err != nil {
			t.Fatalf("reading back the inserted row: %v", err)
		}
		if state != scratchDefaultState {
			t.Errorf("state = %q, want %q. The column has to take its DEFAULT for a role "+
				"that cannot write it; anything else means the insert did not go the way "+
				"registration needs it to", state, scratchDefaultState)
		}
	})

	t.Run("insert_naming_an_ungranted_column_is_refused", func(t *testing.T) {
		_, err := probe.Exec(ctx,
			fmt.Sprintf(`INSERT INTO %s (id, label, state) VALUES ($1, $2, $3)`, table),
			uuid.New(), "second", "verified")

		requireInsufficientPrivilege(t, err,
			"an INSERT NAMING a column the role was not granted was ACCEPTED. The column "+
				"grant is not narrowing anything, and shelter registration could set its "+
				"own status")
	})

	t.Run("update_of_an_ungranted_column_is_refused", func(t *testing.T) {
		_, err := probe.Exec(ctx,
			fmt.Sprintf(`UPDATE %s SET state = $1 WHERE id = $2`, table), "verified", id)

		requireInsufficientPrivilege(t, err,
			"an UPDATE of a column the role was not granted was ACCEPTED. A column grant "+
				"that does not refuse the UPDATE path closes nothing")
	})

	// Anti-vacuity. Without this the three cases above are also satisfied by a
	// role that holds no privilege at all, which is a different schema and a
	// weaker claim: what P2-D4 needs is that the grant NARROWS, not that it
	// refuses everything.
	t.Run("a_granted_column_still_writes", func(t *testing.T) {
		if _, err := probe.Exec(ctx,
			fmt.Sprintf(`UPDATE %s SET label = $1 WHERE id = $2`, table), "renamed", id); err != nil {
			t.Fatalf("updating the GRANTED column: %v. If this fails the other cases prove "+
				"nothing -- they would pass against a role with no privileges whatsoever", err)
		}

		var label string
		if err := probe.QueryRow(ctx,
			fmt.Sprintf(`SELECT label FROM %s WHERE id = $1`, table), id).Scan(&label); err != nil {
			t.Fatalf("reading back the updated row: %v", err)
		}
		if label != "renamed" {
			t.Errorf("label = %q, want %q: the UPDATE reported success without writing", label, "renamed")
		}
	})
}

// scratchDefaultState is the DEFAULT on the column the probe role may not write.
// It stands in for `shelters.status DEFAULT 'pending_verification'`, which is
// the real column this semantic protects.
const scratchDefaultState = "pending"

// newColumnPrivilegeScratch creates a table and a login role that exist only for
// this test, grants the role exactly the column privileges P2-D4's shape needs,
// and returns a pool connected AS that role.
//
// Names carry a random suffix because roles are CLUSTER-wide: a fixed name would
// collide with any other test, or any earlier run against a reused container,
// and the collision would surface as a confusing setup failure rather than as
// what it is.
func newColumnPrivilegeScratch(t *testing.T, env *dbtest.Env) (string, *pgxpool.Pool) {
	t.Helper()

	ctx := context.Background()
	suffix := strings.ReplaceAll(uuid.New().String(), "-", "")[:16]
	table := "column_grant_scratch_" + suffix
	role := "column_grant_probe_" + suffix
	password := "probe-" + suffix

	// NOSUPERUSER is not decoration. A superuser bypasses privilege checks
	// entirely, so every refusal below would stop happening and all four cases
	// would still report a pass -- the test would be measuring nothing.
	dbtest.MustExec(t, env.OwnerPool,
		fmt.Sprintf(`CREATE ROLE %s NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS LOGIN PASSWORD %s`,
			role, quoteLiteral(password)))
	t.Cleanup(func() {
		// Three statements, not two, and the middle one is not obvious.
		//
		// A role cannot be dropped while anything still depends on it, so the
		// table goes first. That is NOT enough: dropping the table removes the
		// grants ON THE TABLE and leaves the `USAGE ON SCHEMA public` grant
		// standing, and PostgreSQL refuses the DROP ROLE with
		// `cannot be dropped because some objects depend on it` (2BP01).
		//
		// `DROP OWNED BY` is the statement that revokes every privilege held by
		// a role. The role owns nothing here, so it drops nothing -- it only
		// clears the grants, which is exactly what is in the way.
		dbtest.MustExec(t, env.OwnerPool, fmt.Sprintf(`DROP TABLE IF EXISTS %s`, table))
		dbtest.MustExec(t, env.OwnerPool, fmt.Sprintf(`DROP OWNED BY %s`, role))
		dbtest.MustExec(t, env.OwnerPool, fmt.Sprintf(`DROP ROLE IF EXISTS %s`, role))
	})

	dbtest.MustExec(t, env.OwnerPool, fmt.Sprintf(`
		CREATE TABLE %s (
		    id    uuid PRIMARY KEY,
		    label text NOT NULL,
		    state text NOT NULL DEFAULT %s
		)`, table, quoteLiteral(scratchDefaultState)))

	dbtest.MustExec(t, env.OwnerPool, fmt.Sprintf(`GRANT USAGE ON SCHEMA public TO %s`, role))
	// SELECT is table-wide so the assertions can read a row back. INSERT and
	// UPDATE are column-scoped and neither one names `state`: that omission is
	// the entire subject of this file.
	dbtest.MustExec(t, env.OwnerPool, fmt.Sprintf(`GRANT SELECT ON %s TO %s`, table, role))
	dbtest.MustExec(t, env.OwnerPool, fmt.Sprintf(`GRANT INSERT (id, label) ON %s TO %s`, table, role))
	dbtest.MustExec(t, env.OwnerPool, fmt.Sprintf(`GRANT UPDATE (label) ON %s TO %s`, table, role))

	// Re-read the attributes rather than trusting the CREATE above. It is the
	// same argument 00013 makes for re-asserting NOSUPERUSER on app_auth: a role
	// that can bypass turns every assertion here into a tautology, and a
	// tautology that reports PASS is worse than no test.
	var superuser, bypassRLS bool
	if err := env.OwnerPool.QueryRow(ctx,
		`SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = $1`, role).
		Scan(&superuser, &bypassRLS); err != nil {
		t.Fatalf("reading the probe role's attributes: %v", err)
	}
	if superuser || bypassRLS {
		t.Fatalf("the probe role is superuser=%v bypassrls=%v; privilege checks would not "+
			"apply to it and every case below would pass without proving anything",
			superuser, bypassRLS)
	}

	dsn, err := url.Parse(env.OwnerDSN)
	if err != nil {
		t.Fatalf("parsing the owner connection string: %v", err)
	}
	dsn.User = url.UserPassword(role, password)

	pool, err := pgxpool.New(ctx, dsn.String())
	if err != nil {
		t.Fatalf("building a pool for the probe role: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("connecting as the probe role: %v", err)
	}

	return table, pool
}

// The refusals above go through `requireInsufficientPrivilege` in scope_test.go
// rather than through a second copy here. It already asserts exactly the right
// thing -- the SQLSTATE and nothing else -- and two helpers spelling the same
// rule are two rules that eventually disagree.

// quoteLiteral wraps a value as a single-quoted SQL literal. Identifiers and
// passwords in CREATE ROLE / DEFAULT clauses cannot be bind parameters -- both
// are utility-statement positions -- and every value passed through here is
// generated in this file from a UUID, never taken from input.
func quoteLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
