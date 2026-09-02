package rlstest_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/gentleman/mascotapp/apps/api/internal/db"
	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
	"github.com/gentleman/mascotapp/apps/api/internal/db/rlstest"
)

// sqlstateReadOnlyTransaction is what a transaction opened with
// pgx.TxOptions{AccessMode: pgx.ReadOnly} raises on a write.
//
// It is named, and kept apart from 42501, because telling the two apart is the
// entire subject of TestPublicWrites_AreRefusedByTheGrantAndNotOnlyByTheTransaction:
// one is the application's doing and one is the database's, and a test that
// accepted either would stop seeing the loss of the second.
const sqlstateReadOnlyTransaction = "25006"

// publicWrites are the three statements the spec names, against the one table
// the public role can actually read. They are deliberately well-formed: a
// statement that failed to parse, or that named a missing column, would be
// refused for a reason that has nothing to do with privileges.
var publicWrites = []struct {
	verb      string
	statement string
	args      func(shelter uuid.UUID) []any
}{
	{
		verb: "INSERT",
		statement: `INSERT INTO pets (id, shelter_id, public_code, name, species_id, size)
		            VALUES ($1, $2, 'P-public-probe', 'Intruder', $3, 'm')`,
		args: func(shelter uuid.UUID) []any {
			return []any{uuid.New(), shelter, uuid.New()}
		},
	},
	{verb: "UPDATE", statement: `UPDATE pets SET name = 'Renamed'`},
	{verb: "DELETE", statement: `DELETE FROM pets`},
}

// The spec's "Public cannot write" scenario, proven at BOTH layers that deliver
// it — because only one of them is the database's, and only the database's
// survives a change of client code.
//
//   - `WithPublic` opens the transaction with pgx.ReadOnly, so any write comes
//     back 25006 before PostgreSQL ever looks at a grant.
//   - `app_public` holds no INSERT/UPDATE/DELETE grant, so a write on an
//     ordinary transaction from the same pool comes back 42501.
//
// NEITHER case pins the grants, and mutation is what settled that rather than
// reasoning about it. Granting `INSERT ON pets TO app_public` leaves both of
// them green: `public_catalog` is `FOR SELECT`, so with the grant in place the
// MISSING INSERT POLICY refuses instead — with the same 42501 a missing grant
// would report. Two different layers, one SQLSTATE, and no behavioural probe can
// tell them apart. Matching on the message text would distinguish them and is
// not worth the brittleness.
//
// So this test asserts what it can actually see, which is still worth asserting:
// that the refusal happens end to end, through the helper every caller goes
// through AND on an ordinary transaction where the read-only mode is not doing
// the work for it. The second case is what proves the database refuses on its
// own — under a `WithPublic` that stopped opening read-only, the write is still
// refused, and that is a property of the schema rather than of the client.
//
// What pins the grants themselves is
// TestPublicRole_HoldsNoWritePrivilegeOnAnyTable, which is therefore
// load-bearing rather than belt-and-braces.
func TestPublicWrites_AreRefusedByTheGrantAndNotOnlyByTheTransaction(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	shelter := freshTenant(t, env)
	pet := seedPet(t, env, shelter)
	publish(t, env, shelter, pet)

	// Anti-vacuity: app_public can READ this pet on an ordinary transaction from
	// its own pool. Without this, a pool that could not connect, or a role that
	// could reach nothing at all, would refuse every statement below and the
	// test would report a guarantee it never exercised.
	if !publiclyVisible(t, env, pet) {
		t.Fatal("app_public cannot see a published, available pet, so every refusal below " +
			"would prove nothing about write privileges specifically")
	}

	for _, w := range publicWrites {
		var args []any
		if w.args != nil {
			args = w.args(shelter)
		}

		t.Run(w.verb+" through WithPublic", func(t *testing.T) {
			err := db.WithPublic(ctx, env.PublicPool,
				func(ctx context.Context, tx pgx.Tx) error {
					_, err := tx.Exec(ctx, w.statement, args...)

					return err
				})
			assertRefusedWith(t, err, sqlstateReadOnlyTransaction, w.verb,
				"WithPublic must open its transaction read-only, so a write is refused "+
					"before any policy or grant is consulted")
		})

		t.Run(w.verb+" on an ordinary transaction", func(t *testing.T) {
			err := publicExec(t, env, w.statement, args)
			assertRefusedWith(t, err, sqlstateInsufficientPrivilege, w.verb,
				"the DATABASE must refuse this on its own. A refusal that depended on "+
					"the read-only transaction would vanish the moment a caller opened a "+
					"read-write one")
		})
	}
}

// Every table, every write privilege, read from the catalog rather than listed.
//
// The grant inventory in isolation_test.go checks hand-picked cells, and hand-
// picked is exactly the problem: it covers the tables somebody remembered on the
// day they wrote it. The public role's contract is not "no writes on these
// tables", it is "no writes, anywhere" — so it is asserted over whatever the
// schema currently holds, and every table a later migration adds is covered on
// the day it lands without anyone extending a list.
//
// TRUNCATE is included deliberately. A mental model built on policies misses it
// entirely: TRUNCATE visits no rows, so no row-level policy can ever see it, and
// the grant is the only thing standing in its way (T-01-020).
func TestPublicRole_HoldsNoWritePrivilegeOnAnyTable(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	tables, err := rlstest.ReadTables(ctx, env.OwnerPool)
	if err != nil {
		t.Fatalf("reading the catalog: %v", err)
	}

	// Anti-vacuity, in both directions. An empty catalog, or a probe that always
	// answered "not granted", would make this test pass over nothing.
	if len(tables) == 0 {
		t.Fatal("the catalog reports no tables at all, so this test asserts nothing")
	}
	if !hasPrivilege(t, env, "app_public", "pets", "SELECT") {
		t.Fatal("app_public does not hold SELECT on pets, so has_table_privilege is not " +
			"reporting what this test reads it as")
	}

	var everWritable bool
	for _, table := range tables {
		for _, privilege := range []string{"INSERT", "UPDATE", "DELETE", "TRUNCATE"} {
			if hasPrivilege(t, env, "app_public", table.Name, privilege) {
				t.Errorf("app_public holds %s on %q. The public role is anonymous: anyone on "+
					"the internet reaches the catalog through it, and a single write bit "+
					"anywhere in the schema is reachable by all of them",
					privilege, table.Name)
			}
		}

		// The same probe, on the same tables, must be able to answer yes.
		if hasPrivilege(t, env, "app_tenant", table.Name, "INSERT") {
			everWritable = true
		}
	}

	if !everWritable {
		t.Fatal("no table in the schema grants INSERT to app_tenant either, which means the " +
			"probe above cannot distinguish a missing grant from a broken query")
	}
}

// LT-2 is NOT enforced by the database, and this test says so out loud.
//
// The plan's §1.1 makes `pending_verification` a hard MVP requirement — *"un
// refugio no puede publicar hasta ser verificado manualmente"* — because an
// unverified shelter publishing an adoption fee is the platform's worst failure
// mode. `shelters.status` does default to 'pending_verification', so the column
// is there. Nothing reads it.
//
// `public_catalog` on `pets` filters on `status`, `published_at` and
// `deleted_at`, and on nothing else. Every public-catalog test in this package
// publishes from a shelter that was never verified and asserts the pet IS
// visible — so the suite currently pins the OPPOSITE of LT-2, which is worth
// knowing before someone reads green as "verification works".
//
// This is a characterisation test, not an endorsement. It fails the day the
// condition lands, with instructions, so the gap cannot be closed halfway and it
// cannot be closed silently either.
//
// It is not fixed here because fixing it is a migration: the condition needs
// `EXISTS (SELECT 1 FROM shelters ...)`, and a policy's subquery requires the
// querying role to hold SELECT on the referenced table (T-01-020, mutant M13).
// `app_public` has no grant and no policy on `shelters`, so adding the condition
// alone would not narrow the catalog — it would break it with a permission error
// on every public read.
func TestPublicCatalog_DoesNotYetEnforceShelterVerification(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	shelter := freshTenant(t, env)

	var status string
	if err := env.OwnerPool.QueryRow(ctx,
		`SELECT status FROM shelters WHERE id = $1`, shelter).Scan(&status); err != nil {
		t.Fatalf("reading the shelter's status: %v", err)
	}
	if status != "pending_verification" {
		t.Fatalf("this shelter is %q, not pending_verification, so the case below says "+
			"nothing about unverified shelters", status)
	}

	pet := seedPet(t, env, shelter)
	publish(t, env, shelter, pet)

	if !publiclyVisible(t, env, pet) {
		t.Fatal("a pet of an UNVERIFIED shelter is no longer in the public catalog. That is " +
			"LT-2 finally being enforced, which is good news and breaks this test on " +
			"purpose. Invert it into the positive assertion — an unverified shelter's pet " +
			"is NOT public, a verified one's IS — and delete this message")
	}

	// The two halves have to land together. A condition without the grant breaks
	// every public read; a grant without the condition is the widest possible
	// state rather than the safest, which is the rule T-01-019 wrote down for
	// `media` and it holds here unchanged.
	filters := policyMentions(t, env, "pets", "public_catalog", "shelters")
	granted := hasPrivilege(t, env, "app_public", "shelters", "SELECT")

	switch {
	case filters && !granted:
		t.Error("public_catalog on pets reads `shelters` but app_public holds no SELECT on " +
			"it. A policy's subquery is filtered by the referenced table's RLS AND needs " +
			"the table privilege, so the public catalog does not narrow — it fails with a " +
			"permission error on every read")
	case !filters && granted:
		t.Error("app_public holds SELECT on `shelters` while nothing narrows what it sees " +
			"there. A grant without its policy is the widest possible state, not the safest")
	}
}

// publicExec runs one statement as app_public on an ORDINARY transaction — not
// through WithPublic, whose read-only access mode would answer first and hide
// whatever the grants say. It always rolls back.
func publicExec(t *testing.T, env *dbtest.Env, statement string, args []any) error {
	t.Helper()

	ctx := context.Background()
	tx, err := env.PublicPool.Begin(ctx)
	if err != nil {
		t.Fatalf("opening a read-write transaction as app_public: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, statement, args...)

	return err
}

// assertRefusedWith fails unless the statement was refused with exactly this
// SQLSTATE. "It failed" is not the assertion: two layers deliver this guarantee
// and they report different codes, so a test that accepted any error would keep
// passing with one of them gone.
func assertRefusedWith(t *testing.T, err error, sqlstate, verb, why string) {
	t.Helper()

	if err == nil {
		t.Fatalf("app_public completed an %s. %s", verb, why)
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("the %s failed with a non-PostgreSQL error, so which layer refused it is "+
			"unknown: %v", verb, err)
	}
	if pgErr.Code != sqlstate {
		t.Fatalf("the %s was refused with %s, not %s. %s (got: %s)",
			verb, pgErr.Code, sqlstate, why, pgErr.Message)
	}
}

// hasPrivilege reads one cell of the grant matrix as the owner.
func hasPrivilege(t *testing.T, env *dbtest.Env, role, table, privilege string) bool {
	t.Helper()

	var granted bool
	if err := env.OwnerPool.QueryRow(context.Background(),
		`SELECT has_table_privilege($1, $2, $3)`,
		role, table, privilege).Scan(&granted); err != nil {
		t.Fatalf("reading %s's %s privilege on %s: %v", role, privilege, table, err)
	}

	return granted
}

// policyMentions reports whether a policy's USING expression references a name.
//
// It reads the rendered expression rather than parsing it, which is enough for
// what it is used for: deciding whether two halves of one change landed
// together, not deciding whether a predicate is correct. Behaviour proves the
// predicate; every other test in this file does that.
func policyMentions(t *testing.T, env *dbtest.Env, table, policy, name string) bool {
	t.Helper()

	var expression *string
	err := env.OwnerPool.QueryRow(context.Background(), `
		SELECT pg_get_expr(p.polqual, p.polrelid)
		FROM pg_policy p
		JOIN pg_class c ON c.oid = p.polrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relname = $1 AND p.polname = $2`,
		table, policy).Scan(&expression)
	if err != nil {
		t.Fatalf("reading policy %s on %s: %v", policy, table, err)
	}
	if expression == nil {
		return false
	}

	return strings.Contains(*expression, name)
}
