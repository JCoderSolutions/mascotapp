package rlstest_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
	"github.com/gentleman/mascotapp/apps/api/internal/db/rlstest"
)

// appendOnlyFixture knows how to put ONE row into an append-only table, because
// each one needs a different parent. Everything else about the case is the same
// for every table, which is the point: the rules are enumerated, the fixtures
// are not.
var appendOnlyFixture = map[string]func(t *testing.T, env *dbtest.Env, shelter uuid.UUID) string{
	"application_events": func(t *testing.T, env *dbtest.Env, shelter uuid.UUID) string {
		t.Helper()

		applicant := uuid.New()
		if err := seedMemberUser(context.Background(), env.OwnerPool, applicant); err != nil {
			t.Fatalf("seeding the applicant: %v", err)
		}
		application := seedApplication(t, env, shelter, seedPet(t, env, shelter), applicant)
		id := seedEvent(t, env, shelter, application)

		return fmt.Sprintf("'%s'::uuid", id)
	},
	"audit_log": func(t *testing.T, env *dbtest.Env, shelter uuid.UUID) string {
		t.Helper()

		return fmt.Sprintf("%d", appendAudit(t, env, shelter, "pet.created"))
	},
}

// The append-only enforcement suite, driven by `Schema.AppendOnly` rather than
// by a list somebody has to remember to extend.
//
// The four layers of the append-only-audit spec each stop a DIFFERENT actor, and
// that is why the suite runs as the OWNER — in this container a superuser, the
// role that holds every privilege and skips every policy:
//
//	revoked grants                  stop the ordinary tenant role
//	absent UPDATE/DELETE policies   stop a restored grant, and the owner
//	the row and statement triggers  stop a role carrying BYPASSRLS
//
// Only the third layer holds against this caller, so a refusal here can only
// come from a trigger — and asserting `P0001` rather than merely "an error"
// is what stops one layer covering for another. On Neon the BYPASSRLS role is
// not hypothetical: `neon_superuser` carries it.
//
// Adding a third append-only table means adding its name to the declaration and
// its fixture above. Nothing else.
func TestAppendOnlyTables_RefuseEveryRewriteEvenForTheOwner(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	verified := 0
	for _, table := range rlstest.Schema.AppendOnly {
		t.Run(table, func(t *testing.T) {
			var exists bool
			if err := env.OwnerPool.QueryRow(ctx,
				`SELECT to_regclass('public.' || $1) IS NOT NULL`, table).Scan(&exists); err != nil {
				t.Fatalf("looking for %s: %v", table, err)
			}
			if !exists {
				t.Skipf("%s has not landed yet (%s)", table, rlstest.Schema.Pending[table])
			}
			verified++

			fixture, ok := appendOnlyFixture[table]
			if !ok {
				t.Fatalf("%s is declared append-only and this suite has no fixture for it, "+
					"so it would be enumerated and never actually probed", table)
			}

			shelter := freshTenant(t, env)
			row := fixture(t, env, shelter)

			// Anti-vacuity, and it is the assertion that separates "append-only"
			// from "nobody can write to this at all". A table the owner cannot
			// append to would pass every refusal below.
			before := rowCount(t, env, table)
			fixture(t, env, shelter)
			if after := rowCount(t, env, table); after != before+1 {
				t.Fatalf("appending to %s moved the row count from %d to %d rather than "+
					"%d, so the refusals below would prove the table is unusable rather "+
					"than append-only", table, before, after, before+1)
			}

			for _, tc := range []struct {
				name      string
				statement string
			}{
				{"update", fmt.Sprintf(
					`UPDATE %s SET shelter_id = shelter_id WHERE id = %s`, table, row)},
				{"delete", fmt.Sprintf(`DELETE FROM %s WHERE id = %s`, table, row)},
			} {
				t.Run(tc.name, func(t *testing.T) {
					_, err := env.OwnerPool.Exec(ctx, tc.statement)
					if err == nil {
						t.Fatalf("a superuser %sd a row of %s. The grant and the missing "+
							"policy both stop app_tenant and neither stops a BYPASSRLS "+
							"role; only a trigger does", tc.name, table)
					}
					assertRaisedByTheTrigger(t, err, tc.name+" on "+table)
				})
			}

			// TRUNCATE removes every row without visiting any, so USING and WITH
			// CHECK are never consulted and a FOR EACH ROW trigger never fires.
			// Only a statement-level trigger reaches it.
			t.Run("truncate", func(t *testing.T) {
				_, err := env.OwnerPool.Exec(ctx, fmt.Sprintf(`TRUNCATE %s`, table))
				if err == nil {
					t.Fatalf("%s was TRUNCATEd. A row-level policy cannot see a truncate at "+
						"all, so a BEFORE TRUNCATE statement trigger is the only thing that "+
						"could have stopped it", table)
				}
				assertRaisedByTheTrigger(t, err, "truncate on "+table)
			})

			// And the row is still there. A refusal that still wrote would be the
			// worst outcome of all.
			if after := rowCount(t, env, table); after != before+1 {
				t.Errorf("%s holds %d rows, want %d. Every statement above was refused and "+
					"one of them went through anyway", table, after, before+1)
			}
		})
	}

	if verified == 0 {
		t.Error("no declared append-only table exists yet, so this suite verified nothing")
	}
}

// The layer that cannot be exercised, because its enforcement is an ABSENCE.
//
// A command with no permissive policy matches zero rows under FORCE, for the
// owner too — but the triggers above refuse first, so no behavioural probe can
// tell an absent UPDATE policy from a present one. The catalog is the only place
// that answer lives, and this is the assertion that keeps the second layer real
// instead of assumed.
func TestAppendOnlyTables_CarryNoMutatingPolicy(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	for _, table := range rlstest.Schema.AppendOnly {
		t.Run(table, func(t *testing.T) {
			rows, err := env.OwnerPool.Query(ctx, `
				SELECT p.polname, p.polcmd
				FROM pg_policy p
				JOIN pg_class c ON c.oid = p.polrelid
				JOIN pg_namespace n ON n.oid = c.relnamespace
				WHERE n.nspname = 'public' AND c.relname = $1
				ORDER BY p.polname COLLATE "C"`, table)
			if err != nil {
				t.Fatalf("reading %s's policies: %v", table, err)
			}
			defer rows.Close()

			commands := map[string]string{}
			for rows.Next() {
				var name, cmd string
				if err := rows.Scan(&name, &cmd); err != nil {
					t.Fatalf("scanning a policy: %v", err)
				}
				commands[cmd] = name
			}
			if err := rows.Err(); err != nil {
				t.Fatalf("reading %s's policies: %v", table, err)
			}

			// `*` is FOR ALL, `w` is UPDATE, `d` is DELETE. Any of the three
			// gives back the command the absence was enforcing.
			for cmd, what := range map[string]string{
				"*": "FOR ALL", "w": "UPDATE", "d": "DELETE",
			} {
				if name, found := commands[cmd]; found {
					t.Errorf("%s carries policy %q covering %s. Append-only rests on that "+
						"policy NOT existing — under FORCE, a command with no permissive "+
						"policy matches zero rows for the owner too. A FOR ALL policy here "+
						"undoes the second of the four layers, and no behavioural test can "+
						"see it because the trigger refuses first", table, name, what)
				}
			}

			// Both halves that MUST be there, or the table is unusable rather
			// than append-only.
			if _, found := commands["r"]; !found {
				t.Errorf("%s carries no SELECT policy, so its rows are unreadable", table)
			}
			if _, found := commands["a"]; !found {
				t.Errorf("%s carries no INSERT policy, so nothing can be appended to it",
					table)
			}
		})
	}
}

// rowCount reads the table as the OWNER, so the count is the real one rather
// than what a policy leaves visible.
func rowCount(t *testing.T, env *dbtest.Env, table string) int {
	t.Helper()

	var n int
	if err := env.OwnerPool.QueryRow(context.Background(),
		fmt.Sprintf(`SELECT count(*) FROM %s`, table)).Scan(&n); err != nil {
		t.Fatalf("counting %s: %v", table, err)
	}

	return n
}
