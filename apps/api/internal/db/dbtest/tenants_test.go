package dbtest_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
)

// A synthetic table, created by the test rather than by a migration, because
// the first real tenant table arrives in T-01-013. The runner is infrastructure
// and its subject here is purpose-built: what is being proven is the runner,
// not the schema.
//
// isolated builds a table with the standard policy template. broken builds the
// same table with a permissive policy, which is what a mistake looks like.
func createTenantTable(t *testing.T, env *dbtest.Env, name string, policies []string) {
	t.Helper()

	ctx := context.Background()
	dbtest.MustExec(t, env.OwnerPool, fmt.Sprintf(`
		CREATE TABLE %s (
			id         uuid NOT NULL PRIMARY KEY,
			shelter_id uuid NOT NULL,
			label      text NOT NULL
		)`, name))
	dbtest.MustExec(t, env.OwnerPool, fmt.Sprintf(`ALTER TABLE %s ENABLE ROW LEVEL SECURITY`, name))
	dbtest.MustExec(t, env.OwnerPool, fmt.Sprintf(`ALTER TABLE %s FORCE ROW LEVEL SECURITY`, name))
	for i, policy := range policies {
		dbtest.MustExec(t, env.OwnerPool, fmt.Sprintf(
			`CREATE POLICY p%d ON %s %s`, i, name, policy))
	}
	dbtest.MustExec(t, env.OwnerPool,
		fmt.Sprintf(`GRANT SELECT, INSERT, UPDATE, DELETE ON %s TO app_tenant`, name))

	t.Cleanup(func() {
		_, _ = env.OwnerPool.Exec(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS %s`, name))
	})
}

// correctPolicies is the standard template every real tenant table uses.
var correctPolicies = []string{
	`FOR ALL TO app_tenant ` +
		`USING (shelter_id = current_setting('app.shelter_id', true)::uuid) ` +
		`WITH CHECK (shelter_id = current_setting('app.shelter_id', true)::uuid)`,
}

func tableCase(name string) dbtest.TenantTable {
	return dbtest.TenantTable{
		Name: name,
		Insert: func(ctx context.Context, tx pgx.Tx, shelterID uuid.UUID) (dbtest.RowKey, error) {
			id := uuid.New()
			_, err := tx.Exec(ctx,
				fmt.Sprintf(`INSERT INTO %s (id, shelter_id, label) VALUES ($1, $2, 'row')`, name),
				id, shelterID)

			return dbtest.RowKey{"id": id}, err
		},
	}
}

// The runner accepts a table whose policy is correct.
func TestTenantTable_Check_PassesForACorrectlyIsolatedTable(t *testing.T) {
	env := dbtest.Postgres(t)
	createTenantTable(t, env, "ab_isolated", correctPolicies)

	if err := tableCase("ab_isolated").Check(context.Background(), env); err != nil {
		t.Fatalf("the runner rejected a correctly isolated table: %v", err)
	}
}

// One broken table is not enough, and mutation testing on 2026-08-30 proved it.
//
// The first version of this test used a single permissive table and asserted
// only that Check returned SOME error. Three mutants survived: disabling the
// read assertion, disabling the forged-insert assertion, and disabling the
// "tenant A can still see its own row" assertion. Each survived because the
// remaining checks still failed on that one table, so the test could not tell
// WHICH check had caught it.
//
// A suite of broken tables only proves that at least one check works. To prove
// each check works, each needs a table broken in exactly the way it exists to
// catch — and none of these is a strawman. Every one is a policy somebody writes
// on the way to the right answer.
func TestTenantTable_Check_FailsForEachDistinctWayIsolationBreaks(t *testing.T) {
	env := dbtest.Postgres(t)

	const scoped = `shelter_id = current_setting('app.shelter_id', true)::uuid`

	cases := []struct {
		name string
		// policies is the complete policy set for the table.
		policies []string
		// wants is a fragment the failure must mention, so the test proves the
		// RIGHT check fired rather than merely that something did.
		wants string
		why   string
	}{
		{
			name:     "permissive_policy",
			policies: []string{`FOR ALL TO app_tenant USING (true) WITH CHECK (true)`},
			wants:    "can READ",
			why:      "what a policy looks like when it is loosened for debugging and left that way",
		},
		{
			// A permissive DELETE policy behind a CORRECT select policy. The
			// qualified DELETE assertion cannot see this: PostgreSQL applies the
			// SELECT policy in addition to the DELETE policy whenever a statement
			// reads columns, so `DELETE ... WHERE` is stopped by the SELECT
			// policy and the DELETE policy is never exercised. Only the
			// unqualified probe reaches it.
			//
			// It is also the most expensive failure in the set: tenant B cannot
			// see tenant A's rows and can destroy every one of them.
			name: "deletable_but_not_readable",
			policies: []string{
				`FOR INSERT TO app_tenant WITH CHECK (` + scoped + `)`,
				`FOR SELECT TO app_tenant USING (` + scoped + `)`,
				`FOR UPDATE TO app_tenant USING (` + scoped + `) WITH CHECK (` + scoped + `)`,
				`FOR DELETE TO app_tenant USING (true)`,
			},
			wants: "unqualified DELETE",
			why: "lets tenant B wipe tenant A's rows with `DELETE FROM t` while hiding " +
				"those same rows from every SELECT",
		},
		{
			name: "no_with_check",
			policies: []string{
				`FOR ALL TO app_tenant USING (` + scoped + `) WITH CHECK (true)`,
			},
			wants: "stamped with tenant A's shelter_id",
			why: "filters reads correctly and lets tenant B write a row it will never be " +
				"able to see: the single easiest RLS mistake to ship",
		},
		{
			name: "hidden_from_its_own_owner",
			policies: []string{
				`FOR INSERT TO app_tenant WITH CHECK (` + scoped + `)`,
				`FOR SELECT TO app_tenant USING (false)`,
				`FOR UPDATE TO app_tenant USING (false)`,
				`FOR DELETE TO app_tenant USING (false)`,
			},
			wants: "can no longer see its own row",
			why: "isolates perfectly and is still broken; \"nobody can read it\" passes " +
				"every cross-tenant assertion",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			table := "ab_" + tc.name
			createTenantTable(t, env, table, tc.policies)

			err := tableCase(table).Check(context.Background(), env)
			if err == nil {
				t.Fatalf("the runner accepted a table that %s. Every A/B case in this "+
					"phase would then pass for the wrong reason", tc.why)
			}
			if !strings.Contains(err.Error(), tc.wants) {
				t.Fatalf("the runner failed, but not on the check this table breaks.\n"+
					"  want a failure mentioning: %q\n  got: %v", tc.wants, err)
			}
			t.Logf("correctly refused: %v", err)
		})
	}
}

// The wipe probe measures against the rows tenant B can see, not against zero,
// and this is the case that forces the difference.
//
// `shelters` is where it stops being hypothetical: tenant B owns a shelters row,
// because every other table's foreign keys need it to exist. An assertion of
// "the unqualified DELETE removed nothing" would fail there on correct
// behaviour, and the obvious repair — dropping the shelters case — would drop
// the only assertion in the suite that ever reaches a DELETE policy.
func TestTenantTable_Check_AllowsTenantBToDeleteItsOwnRows(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	createTenantTable(t, env, "ab_b_owns_rows", correctPolicies)

	// Seed a row for tenant B, which is what the runner never does on its own.
	dbtest.MustExec(t, env.OwnerPool,
		`INSERT INTO ab_b_owns_rows (id, shelter_id, label) VALUES ($1, $2, 'b')`,
		uuid.New(), env.ShelterB)

	if err := tableCase("ab_b_owns_rows").Check(ctx, env); err != nil {
		t.Fatalf("the runner refused a correctly isolated table because tenant B was able "+
			"to delete its OWN row, which is not a leak: %v", err)
	}
}

// pet_media has no `id` column: §4.3 gives it `(pet_id, media_id, position,
// is_primary)`, so its row identity is a pair. The runner has to build its WHERE
// clause from the key it is given rather than assuming a single column.
func TestTenantTable_Check_HandlesACompositeRowIdentity(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	dbtest.MustExec(t, env.OwnerPool, `
		CREATE TABLE ab_pair (
			left_id    uuid NOT NULL,
			right_id   uuid NOT NULL,
			shelter_id uuid NOT NULL,
			PRIMARY KEY (left_id, right_id)
		)`)
	dbtest.MustExec(t, env.OwnerPool, `ALTER TABLE ab_pair ENABLE ROW LEVEL SECURITY`)
	dbtest.MustExec(t, env.OwnerPool, `ALTER TABLE ab_pair FORCE ROW LEVEL SECURITY`)
	dbtest.MustExec(t, env.OwnerPool, `CREATE POLICY tenant_isolation ON ab_pair FOR ALL
		TO app_tenant
		USING (shelter_id = current_setting('app.shelter_id', true)::uuid)
		WITH CHECK (shelter_id = current_setting('app.shelter_id', true)::uuid)`)
	dbtest.MustExec(t, env.OwnerPool,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ab_pair TO app_tenant`)
	t.Cleanup(func() { _, _ = env.OwnerPool.Exec(ctx, `DROP TABLE IF EXISTS ab_pair`) })

	table := dbtest.TenantTable{
		Name: "ab_pair",
		Insert: func(ctx context.Context, tx pgx.Tx, shelterID uuid.UUID) (dbtest.RowKey, error) {
			left, right := uuid.New(), uuid.New()
			_, err := tx.Exec(ctx,
				`INSERT INTO ab_pair (left_id, right_id, shelter_id) VALUES ($1, $2, $3)`,
				left, right, shelterID)

			return dbtest.RowKey{"left_id": left, "right_id": right}, err
		},
	}

	if err := table.Check(ctx, env); err != nil {
		t.Fatalf("the runner could not handle a composite row identity: %v", err)
	}
}

// The two shelters must be distinct and both non-nil, or every A/B assertion
// compares a tenant with itself and passes for free.
func TestEnv_ProvidesTwoDistinctShelters(t *testing.T) {
	env := dbtest.Postgres(t)

	if env.ShelterA == uuid.Nil || env.ShelterB == uuid.Nil {
		t.Fatalf("shelters must be non-nil: A=%v B=%v", env.ShelterA, env.ShelterB)
	}
	if env.ShelterA == env.ShelterB {
		t.Fatal("A and B are the same shelter, so every isolation assertion is vacuous")
	}
}

// The wipe probe deliberately attempts a destructive statement, so it must never
// keep the result. Mutation testing found that nothing enforced it: making the
// probe commit instead of roll back broke no test, because on a correctly
// isolated table the delete removes nothing anyway.
//
// It only bites on the tables where the runner reports a problem — which is
// exactly when the evidence matters most. A harness that proves data can be
// destroyed by destroying it is not a harness.
func TestTenantTable_Check_WipeProbeLeavesTheDataIntact(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	const scoped = `shelter_id = current_setting('app.shelter_id', true)::uuid`
	createTenantTable(t, env, "ab_wipe_probe", []string{
		`FOR INSERT TO app_tenant WITH CHECK (` + scoped + `)`,
		`FOR SELECT TO app_tenant USING (` + scoped + `)`,
		`FOR UPDATE TO app_tenant USING (` + scoped + `) WITH CHECK (` + scoped + `)`,
		`FOR DELETE TO app_tenant USING (true)`, // the hole the probe finds
	})

	if err := tableCase("ab_wipe_probe").Check(ctx, env); err == nil {
		t.Fatal("expected the runner to reject this table")
	}

	// Read as the owner: the point is what is on disk, not what a policy shows.
	var remaining int
	if err := env.OwnerPool.QueryRow(ctx,
		`SELECT count(*) FROM ab_wipe_probe`).Scan(&remaining); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	if remaining == 0 {
		t.Fatal("the wipe probe committed: proving the rows could be destroyed destroyed them")
	}
}
