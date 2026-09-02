package rlstest_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/gentleman/mascotapp/apps/api/internal/db"
	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
	"github.com/gentleman/mascotapp/apps/api/internal/db/rlstest"
)

// orphanCase is one child table's minimal insert, parameterised by exactly the
// pair its composite foreign key checks. Everything else on the row is left at
// its default so that nothing but that pair can decide the outcome.
type orphanCase struct {
	table string
	build func(shelter, pet, media uuid.UUID) (string, []any)
}

var petChildren = []orphanCase{
	{
		table: "pet_media",
		build: func(shelter, pet, media uuid.UUID) (string, []any) {
			return `INSERT INTO pet_media (pet_id, media_id, shelter_id, position)
			        VALUES ($1, $2, $3, 0)`, []any{pet, media, shelter}
		},
	},
	{
		table: "pet_health_records",
		build: func(shelter, pet, _ uuid.UUID) (string, []any) {
			return `INSERT INTO pet_health_records
			            (id, shelter_id, pet_id, type, occurred_on)
			        VALUES ($1, $2, $3, 'checkup', current_date)`,
				[]any{uuid.New(), shelter, pet}
		},
	},
	{
		table: "pet_status_history",
		build: func(shelter, pet, _ uuid.UUID) (string, []any) {
			return `INSERT INTO pet_status_history
			            (id, shelter_id, pet_id, from_status, to_status)
			        VALUES ($1, $2, $3, 'draft', 'available')`,
				[]any{uuid.New(), shelter, pet}
		},
	},
}

// The assertion that proves D5, and the one the read-path tests structurally
// cannot make.
//
// PostgreSQL documents that referential integrity checks ALWAYS BYPASS ROW
// SECURITY: they run as the constraint's owner, not as the querying role. So a
// plain `pet_id REFERENCES pets (id)` would happily resolve tenant A's pet on
// behalf of tenant B — B cannot SELECT that row, and would be linking to it
// anyway. Only the composite key to `pets (id, shelter_id)` makes the pair
// unresolvable, and nothing that READS can notice its absence.
//
// Note carefully which cross-tenant write this is. The A/B suite already covers
// the other one: a child stamped with tenant A's `shelter_id`, refused by the
// policy's WITH CHECK with 42501. Here the row carries B's OWN `shelter_id`, so
// the policy is satisfied and steps aside — which is the entire point. A 42501
// in this test is a FAILING result, not a passing one: it means the policy got
// there first and the foreign key was never exercised, so the key could be
// dropped tomorrow and this test would keep reporting a refusal.
//
// And the refusal alone is not the property. Two DISTINGUISHABLE refusals are
// still a leak: if a foreign parent answered differently from a nonexistent
// one, B could enumerate another shelter's pets by reading the difference off
// the error. The property is that the two are the SAME answer.
func TestChildTables_CannotReferenceAnotherTenantsPet(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	tenantA, tenantB := freshTenant(t, env), freshTenant(t, env)
	petOfA := seedPet(t, env, tenantA)
	petOfB := seedPet(t, env, tenantB)
	mediaOfB := seedMedia(t, env, tenantB)

	for _, tc := range petChildren {
		t.Run(tc.table, func(t *testing.T) {
			// Anti-vacuity. A table B cannot write to at all would refuse both
			// cases below and read as perfectly isolated. This also proves every
			// OTHER constraint on the row is satisfiable with these values, so a
			// refusal below can only be about the parent.
			sql, args := tc.build(tenantB, petOfB, mediaOfB)
			if err := db.WithTenant(ctx, env.TenantPool, tenantB,
				func(ctx context.Context, tx pgx.Tx) error {
					_, err := tx.Exec(ctx, sql, args...)

					return err
				}); err != nil {
				t.Fatalf("tenant B could not attach a row of %s to its OWN pet, so both "+
					"refusals below would prove nothing about tenant isolation: %v",
					tc.table, err)
			}

			sql, args = tc.build(tenantB, petOfA, mediaOfB)
			foreign := refusal(t, env, tenantB, sql, args,
				"tenant B inserted a row of "+tc.table+" naming tenant A's pet")

			sql, args = tc.build(tenantB, uuid.New(), mediaOfB)
			absent := refusal(t, env, tenantB, sql, args,
				"tenant B inserted a row of "+tc.table+" naming a pet that does not exist")

			if foreign.Code == sqlstateInsufficientPrivilege {
				t.Fatalf("the cross-tenant insert into %s was refused by the POLICY (%s), not "+
					"by a foreign key. The row carries B's own shelter_id, so WITH CHECK "+
					"should have passed and the composite key should have done the refusing. "+
					"As written this test cannot see the key disappear: %s",
					tc.table, sqlstateInsufficientPrivilege, foreign.Message)
			}
			if foreign.Code != sqlstateForeignKeyViolation {
				t.Fatalf("naming tenant A's pet in %s was refused with %s, not with a foreign "+
					"key violation (%s), so what refused it is unproven: %s",
					tc.table, foreign.Code, sqlstateForeignKeyViolation, foreign.Message)
			}

			// The oracle-closing assertion. Everything above only establishes
			// that both attempts failed; this is the one that says B learned
			// nothing by trying.
			if foreign.Code != absent.Code || foreign.ConstraintName != absent.ConstraintName {
				t.Errorf("%s answers differently for another tenant's pet (%s/%s) than for a "+
					"pet that does not exist (%s/%s). Tenant B can tell the two apart, which "+
					"means it can enumerate tenant A's pets one id at a time — an answer to a "+
					"question it has no right to ask",
					tc.table,
					foreign.Code, foreign.ConstraintName,
					absent.Code, absent.ConstraintName)
			}
			if foreign.TableName != tc.table {
				t.Errorf("the refusal was reported against %q rather than %q, so the "+
					"constraint that fired is not the one this case is about",
					foreign.TableName, tc.table)
			}
		})
	}
}

// `pet_media` carries TWO references, and the second one is the easy one to
// miss for exactly the reason T-01-020 found on `pet_health_records`: the key to
// `pets` is the one D5 writes out, so a single-column `REFERENCES media (id)`
// alongside it looks finished.
//
// It leaks the same way and it leaks about a different table. A shelter's media
// keys are not public, so distinguishing "that photo belongs to someone else"
// from "that photo does not exist" hands tenant B a probe over tenant A's
// storage.
func TestPetMedia_CannotReferenceAnotherTenantsMedia(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	tenantA, tenantB := freshTenant(t, env), freshTenant(t, env)
	mediaOfA := seedMedia(t, env, tenantA)
	petOfB := seedPet(t, env, tenantB)

	attach := func(media uuid.UUID) (string, []any) {
		return `INSERT INTO pet_media (pet_id, media_id, shelter_id, position)
		        VALUES ($1, $2, $3, 0)`, []any{petOfB, media, tenantB}
	}

	// Anti-vacuity: B can attach its own photo to its own pet.
	sql, args := attach(seedMedia(t, env, tenantB))
	if err := db.WithTenant(ctx, env.TenantPool, tenantB,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, sql, args...)

			return err
		}); err != nil {
		t.Fatalf("tenant B could not attach its OWN photo to its OWN pet, so the refusals "+
			"below prove nothing: %v", err)
	}

	sql, args = attach(mediaOfA)
	foreign := refusal(t, env, tenantB, sql, args,
		"tenant B attached tenant A's photo to its own pet")

	sql, args = attach(uuid.New())
	absent := refusal(t, env, tenantB, sql, args,
		"tenant B attached a photo that does not exist")

	if foreign.Code != sqlstateForeignKeyViolation {
		t.Fatalf("naming tenant A's media was refused with %s rather than a foreign key "+
			"violation (%s): %s", foreign.Code, sqlstateForeignKeyViolation, foreign.Message)
	}
	if foreign.Code != absent.Code || foreign.ConstraintName != absent.ConstraintName {
		t.Errorf("pet_media answers differently for another tenant's media (%s/%s) than for "+
			"media that does not exist (%s/%s), so tenant B can probe tenant A's storage one "+
			"id at a time",
			foreign.Code, foreign.ConstraintName, absent.Code, absent.ConstraintName)
	}
}

// refusal runs one statement as the given tenant and returns the PgError it came
// back with. A statement that SUCCEEDS fails the test here rather than at the
// call site, because an accepted cross-tenant write is the leak this whole file
// exists to catch and there is nothing left to compare afterwards.
func refusal(
	t *testing.T, env *dbtest.Env, shelter uuid.UUID, sql string, args []any, what string,
) *pgconn.PgError {
	t.Helper()

	err := db.WithTenant(context.Background(), env.TenantPool, shelter,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, sql, args...)

			return err
		})
	if err == nil {
		t.Fatalf("%s, and the database accepted it. Referential checks bypass row security, "+
			"so a single-column reference resolves another tenant's row on the attacker's "+
			"behalf; only the composite key to (id, shelter_id) makes the pair unresolvable "+
			"(D5)", what)
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("%s and failed, but not with a PostgreSQL error, so its SQLSTATE cannot be "+
			"compared against the other case: %v", what, err)
	}

	return pgErr
}

// freshTenant creates a shelter this test alone owns.
//
// It deliberately does NOT reuse `env.ShelterA` / `env.ShelterB`. Those belong
// to the A/B runner, and borrowing them from an unrelated test breaks the runner
// in two different ways — both found by running this file, which sorts first in
// the package and therefore seeds before `TestTenantIsolation` does.
//
//   - The `shelters` case has `TenantColumn: "id"`, so tenant A's row IS shelter
//     A and the case has to be the one that CREATES it. A test that seeds it
//     first turns that insert into a duplicate key.
//   - The `pets` case probes an unqualified `DELETE FROM pets` as tenant B — the
//     only shape that can catch a permissive DELETE policy. Any leftover pet of
//     B carrying status history refuses it outright through `ON DELETE
//     RESTRICT`, failing a case that has nothing to do with this file.
//
// Same lesson twice: shared fixtures in a shared container are a coupling, and a
// test that needs its own tenants mints them rather than reaching for someone
// else's.
func freshTenant(t *testing.T, env *dbtest.Env) uuid.UUID {
	t.Helper()

	id := uuid.New()
	if err := seedShelters(context.Background(), env.OwnerPool, id); err != nil {
		t.Fatalf("creating a shelter for this test: %v", err)
	}

	return id
}

// seedMedia creates one media row for a shelter, as that tenant, and returns its
// id. The kind is an image because every caller here attaches it to a pet.
func seedMedia(t *testing.T, env *dbtest.Env, shelter uuid.UUID) uuid.UUID {
	t.Helper()

	id := uuid.New()
	err := db.WithTenant(context.Background(), env.TenantPool, shelter,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO media (id, shelter_id, kind, storage_key, mime, bytes)
				 VALUES ($1, $2, 'image', $3, 'image/avif', 2048)`,
				id, shelter, "shelters/"+shelter.String()+"/"+id.String())

			return err
		})
	if err != nil {
		t.Fatalf("seeding media for %s: %v", shelter, err)
	}

	return id
}

// The structural half of D5, driven by the DECLARATION rather than by a list
// somebody has to remember to extend.
//
// This exists because the hand-written list is exactly how `form_submissions`
// slipped through. T-01-025 shipped it with a working composite key, and a
// mutant that reduced that key to `REFERENCES form_template_versions (id)`
// SURVIVED the whole suite: the orphan cases above enumerate `pets`' children by
// hand, and the parent-key inventory in media_test.go looks at parents. Nothing
// asked the question from the child's side.
//
// So the question is asked here, over `Schema.TenantChildren`, and the day
// `adoption_applications`, `application_events`, `application_notes` or
// `documents` land, their key is checked the moment the table appears — no test
// to remember to write.
//
// It is structural, not behavioural, and that is deliberate. The behavioural
// cases above are the ones that prove the refusal is INDISTINGUISHABLE; this one
// proves the constraint exists at all, which is the property a later "tidying"
// commit removes.
func TestTenantChildren_ReferenceTheirParentCompositely(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	verified := 0
	for _, child := range rlstest.Schema.TenantChildren {
		t.Run(child, func(t *testing.T) {
			var exists bool
			if err := env.OwnerPool.QueryRow(ctx,
				`SELECT to_regclass('public.' || $1) IS NOT NULL`, child).Scan(&exists); err != nil {
				t.Fatalf("looking for %s: %v", child, err)
			}
			if !exists {
				t.Skipf("%s has not landed yet (%s); a constraint assertion over a table "+
					"that does not exist is a no-op", child, rlstest.Schema.Pending[child])
			}
			verified++

			// EVERY reference to a tenant table, not merely one of them. The
			// weaker form of this query — "does the table have A composite
			// reference" — is what let T-01-027's mutant M10 through: reducing
			// `form_submissions.application_id` to a single-column key survived,
			// because the table still had its composite key to the version.
			//
			// The one exemption is the table's own `shelter_id REFERENCES
			// shelters (id)`, whose key is exactly `shelter_id` and which is what
			// makes the column mean anything in the first place.
			rows, err := env.OwnerPool.Query(ctx, `
				SELECT c.conname, parent.relname
				FROM pg_constraint c
				JOIN pg_class parent ON parent.oid = c.confrelid
				WHERE c.conrelid = ('public.' || $1)::regclass
				  AND c.contype = 'f'
				  AND parent.relname = ANY ($2)
				  AND c.conkey <> ARRAY[(
				        SELECT a.attnum FROM pg_attribute a
				        WHERE a.attrelid = c.conrelid AND a.attname = 'shelter_id')]
				  AND NOT (
				        SELECT a.attnum FROM pg_attribute a
				        WHERE a.attrelid = c.conrelid AND a.attname = 'shelter_id'
				      ) = ANY (c.conkey)`, child, rlstest.Schema.Tenant)
			if err != nil {
				t.Fatalf("reading %s's foreign keys: %v", child, err)
			}
			defer rows.Close()

			for rows.Next() {
				var name, parent string
				if err := rows.Scan(&name, &parent); err != nil {
					t.Fatalf("scanning a foreign key: %v", err)
				}
				t.Errorf("%s.%s references the tenant table %q without `shelter_id` in the "+
					"key. Referential integrity checks ALWAYS BYPASS ROW SECURITY, so it "+
					"resolves another shelter's %s on this tenant's behalf — and nothing "+
					"that READS can notice (D5). It must be the composite key to "+
					"(id, shelter_id)", child, name, parent, parent)
			}
			if err := rows.Err(); err != nil {
				t.Fatalf("reading %s's foreign keys: %v", child, err)
			}

			// And at least one composite reference must exist at all, or a child
			// that lost every key would pass the loop above by having nothing to
			// iterate.
			var composite bool
			if err := env.OwnerPool.QueryRow(ctx, `
				SELECT EXISTS (
					SELECT 1
					FROM pg_constraint c
					WHERE c.conrelid = ('public.' || $1)::regclass
					  AND c.contype = 'f'
					  AND array_length(c.conkey, 1) >= 2
					  AND (
					        SELECT a.attnum FROM pg_attribute a
					        WHERE a.attrelid = c.conrelid AND a.attname = 'shelter_id'
					      ) = ANY (c.conkey)
				)`, child).Scan(&composite); err != nil {
				t.Fatalf("reading %s's foreign keys: %v", child, err)
			}
			if !composite {
				t.Errorf("%s is declared a tenant child and carries no composite reference "+
					"at all. Its shelter_id is denormalised from a parent it no longer "+
					"names (D5)", child)
			}
		})
	}

	if verified == 0 {
		t.Error("no declared tenant child exists yet, so this test verified nothing. A " +
			"suite full of silent no-ops reads as proof while proving nothing")
	}
}
