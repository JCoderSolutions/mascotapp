package rlstest_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/gentleman/mascotapp/apps/api/internal/db"
	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
)

const sqlstateUniqueViolation = "23505"

// A column the schema declares NOT NULL, refused. Distinct from a foreign key
// violation on purpose: `documents.media_id` is BOTH, and only the SQLSTATE
// says which layer answered.
const sqlstateNotNullViolation = "23502"

// seedTemplate creates one form template for a shelter, as that tenant.
func seedTemplate(t *testing.T, env *dbtest.Env, shelter uuid.UUID, key string) uuid.UUID {
	t.Helper()

	id := uuid.New()
	if err := insertTemplate(env, shelter, id, key); err != nil {
		t.Fatalf("seeding template %q for %s: %v", key, shelter, err)
	}

	return id
}

func insertTemplate(env *dbtest.Env, shelter, id uuid.UUID, key string) error {
	return db.WithTenant(context.Background(), env.TenantPool, shelter,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO form_templates (id, shelter_id, key, name, purpose)
				 VALUES ($1, $2, $3, 'Adoption form', 'adoption_application')`,
				id, shelter, key)

			return err
		})
}

// seedVersion appends one version to a template. `published` decides whether the
// row is frozen on arrival.
func seedVersion(
	t *testing.T, env *dbtest.Env, shelter, template uuid.UUID, version int, published bool,
) uuid.UUID {
	t.Helper()

	id := uuid.New()
	publishedAt := "NULL"
	if published {
		publishedAt = "now()"
	}

	err := db.WithTenant(context.Background(), env.TenantPool, shelter,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO form_template_versions
				     (id, shelter_id, template_id, version, definition, published_at)
				 VALUES ($1, $2, $3, $4, $5::jsonb, `+publishedAt+`)`,
				id, shelter, template, version, `{"schemaVersion":1,"sections":[]}`)

			return err
		})
	if err != nil {
		t.Fatalf("seeding version %d of template %s: %v", version, template, err)
	}

	return id
}

// The spec's immutability requirement, and the reason it lives in the database:
// *"Once a form_template_versions row has a non-null published_at, its
// definition and version MUST NOT change, and the row MUST NOT be deleted."*
//
// It runs as the OWNER, which in this container is a superuser. Unlike
// `pet_status_history`, immutability here is CONDITIONAL — draft rows stay fully
// editable — so it cannot be delivered by revoking UPDATE and DELETE. A trigger
// is the only mechanism that can decide per row, and a trigger is also the only
// layer that survives a role with BYPASSRLS (`neon_superuser` carries it). If
// these refusals hold for a superuser, the trigger is what is doing the work.
//
// The `published_at` case is the one that is easy to leave out and is the whole
// door: a trigger that guarded only `definition` and `version` would let a
// shelter clear `published_at`, edit freely, and republish — immutability
// bypassed without ever touching a guarded column. Same shape as the cascading
// foreign key in T-01-020: the hole is never in the column you were watching.
func TestPublishedFormVersion_CannotBeEditedOrDeleted(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	shelter := freshTenant(t, env)
	template := seedTemplate(t, env, shelter, "adoption")

	// Anti-vacuity first: a DRAFT version is fully editable and deletable. This
	// is what separates "published rows are frozen" from "this table rejects
	// every write", which would pass every case below while breaking the
	// product — a form nobody can edit before publishing it is not a form.
	t.Run("a draft version is still editable", func(t *testing.T) {
		draft := seedVersion(t, env, shelter, template, 1, false)

		// RowsAffected, not just the absence of an error. A BEFORE row trigger
		// that returns NULL CANCELS its statement SILENTLY -- no error, zero
		// rows -- so "err == nil" would report a draft as editable while the
		// trigger quietly discarded every change.
		tag, err := env.OwnerPool.Exec(ctx,
			`UPDATE form_template_versions SET definition = '{"schemaVersion":2}'::jsonb
			 WHERE id = $1`, draft)
		if err != nil {
			t.Fatalf("a draft version could not be edited: %v", err)
		}
		if tag.RowsAffected() != 1 {
			t.Fatalf("editing a draft version reported no error and touched %d rows. A "+
				"BEFORE trigger returning NULL cancels the statement without saying so",
				tag.RowsAffected())
		}

		tag, err = env.OwnerPool.Exec(ctx,
			`DELETE FROM form_template_versions WHERE id = $1`, draft)
		if err != nil {
			t.Fatalf("a draft version could not be deleted: %v", err)
		}
		if tag.RowsAffected() != 1 {
			t.Fatalf("deleting a draft version reported no error and removed %d rows",
				tag.RowsAffected())
		}
	})

	published := seedVersion(t, env, shelter, template, 2, true)

	for _, tc := range []struct {
		name      string
		statement string
		why       string
	}{
		{
			"editing the definition",
			`UPDATE form_template_versions SET definition = '{"schemaVersion":9}'::jsonb
			 WHERE id = $1`,
			"every submission recorded against this version becomes unreadable",
		},
		{
			"renumbering the version",
			`UPDATE form_template_versions SET version = 99 WHERE id = $1`,
			"a submission points at a version number; moving it orphans the answer",
		},
		{
			"clearing published_at",
			`UPDATE form_template_versions SET published_at = NULL WHERE id = $1`,
			"unpublishing is the bypass: thaw the row, edit it, publish again, and " +
				"immutability was never enforced at all",
		},
		{
			"deleting the row",
			`DELETE FROM form_template_versions WHERE id = $1`,
			"the spec says the row must not be deleted; a dangling submission is worse " +
				"than a stale one",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := env.OwnerPool.Exec(ctx, tc.statement, published)
			if err == nil {
				t.Fatalf("a superuser succeeded at %s on a PUBLISHED version. %s",
					tc.name, tc.why)
			}
			assertRefusedWith(t, err, sqlstateRaiseException, tc.name,
				"immutability must come from a trigger, which is the only layer that "+
					"decides per row AND survives a BYPASSRLS role")
		})
	}

	// TRUNCATE takes the published rows with it and no row-level trigger sees
	// it: it removes every row without visiting any. It has to be refused
	// unconditionally, because it cannot be refused selectively.
	t.Run("truncating the table", func(t *testing.T) {
		// A plain TRUNCATE stopped reaching the trigger the moment
		// `form_submissions` started referencing this table (T-01-025):
		// PostgreSQL refuses it outright with 0A000, "cannot truncate a table
		// referenced in a foreign key constraint". That is a real extra layer
		// and it is NOT the one under test — it disappears the day the last
		// reference does.
		if _, err := env.OwnerPool.Exec(ctx, `TRUNCATE form_template_versions`); err == nil {
			t.Fatal("form_template_versions was TRUNCATEd, published rows and all")
		}

		// CASCADE is the bypass of that layer, which is exactly why the trigger
		// has to exist. Anyone who hits the 0A000 above reaches for this next.
		_, err := env.OwnerPool.Exec(ctx, `TRUNCATE form_template_versions CASCADE`)
		if err == nil {
			t.Fatal("form_template_versions was TRUNCATEd with CASCADE, published rows and " +
				"all. CASCADE walks straight past the foreign key that refuses a plain " +
				"truncate, and a BEFORE TRUNCATE statement trigger is the only thing left")
		}
		assertRefusedWith(t, err, sqlstateRaiseException, "cascading truncate",
			"only a statement-level trigger can see a truncate at all")
	})

	// The spec asks for it explicitly: *"AND the stored definition is
	// unchanged"*. A refusal that still wrote would be the worst outcome of all.
	var schemaVersion int
	if err := env.OwnerPool.QueryRow(ctx,
		`SELECT (definition ->> 'schemaVersion')::int
		 FROM form_template_versions WHERE id = $1`, published).Scan(&schemaVersion); err != nil {
		t.Fatalf("reading the published definition back: %v", err)
	}
	if schemaVersion != 1 {
		t.Errorf("the published definition is now schemaVersion %d, not 1. Every statement "+
			"above was refused and one of them wrote anyway", schemaVersion)
	}
}

// A template key is a shelter's own label — `adoption`, `home_visit` — and two
// shelters naturally pick the same words. `UNIQUE (shelter_id, key)` is what the
// spec asks for; this asserts both halves, because a global key would be both
// wrong for the product AND a cross-tenant existence oracle.
func TestFormTemplateKeys_AreUniquePerShelterNotGlobally(t *testing.T) {
	env := dbtest.Postgres(t)

	shelterA, shelterB := freshTenant(t, env), freshTenant(t, env)
	seedTemplate(t, env, shelterA, "adoption")

	// Shelter B uses the same word. This must succeed.
	if err := insertTemplate(env, shelterB, uuid.New(), "adoption"); err != nil {
		t.Fatalf("shelter B could not create its own template keyed 'adoption'. Every "+
			"shelter names its adoption form the obvious thing; a global key makes the "+
			"second shelter to sign up unable to: %v", err)
	}

	// The same shelter, twice. This must not.
	err := insertTemplate(env, shelterA, uuid.New(), "adoption")
	if err == nil {
		t.Fatal("shelter A holds two templates keyed 'adoption'. The key is how the " +
			"application addresses a form, so a duplicate makes which one it resolves a " +
			"matter of planner order")
	}
	assertRefusedWith(t, err, sqlstateUniqueViolation, "the duplicate key",
		"UNIQUE (shelter_id, key) is what refuses it")
}

// `UNIQUE (template_id, version)` is what the spec writes, and it is the third
// existence oracle this phase has had to close — after `pets.microchip_id`
// (T-01-019) and `pet_media`'s cover photo (T-01-020).
//
// Verified against PostgreSQL 17 rather than assumed: the UNIQUE INDEX IS
// CHECKED BEFORE THE FOREIGN KEY. Referential checks run as AFTER triggers,
// while the index insert happens on the heap write. So with a global key, tenant
// B — stamped with its OWN shelter_id, which satisfies the policy — could name
// tenant A's template and read the answer off the SQLSTATE:
//
//	23505  that template already has that version
//	23503  it does not, or it is not B's
//
// Adding `shelter_id` to the key keeps the guarantee identical inside a tenant —
// a template belongs to exactly one shelter, so per-shelter uniqueness over its
// versions IS per-template uniqueness — while collapsing the two answers into
// one for everybody else.
func TestFormTemplateVersions_VersionKeyIsNotACrossTenantOracle(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	shelterA, shelterB := freshTenant(t, env), freshTenant(t, env)
	templateOfA := seedTemplate(t, env, shelterA, "adoption")
	seedVersion(t, env, shelterA, templateOfA, 1, true)

	templateOfB := seedTemplate(t, env, shelterB, "adoption")

	// Anti-vacuity: B can version its OWN template. Otherwise a table B could
	// not write to at all would answer both probes identically and pass.
	seedVersion(t, env, shelterB, templateOfB, 1, false)

	attach := func(version int) error {
		return db.WithTenant(ctx, env.TenantPool, shelterB,
			func(ctx context.Context, tx pgx.Tx) error {
				_, err := tx.Exec(ctx,
					`INSERT INTO form_template_versions
					     (id, shelter_id, template_id, version, definition)
					 VALUES ($1, $2, $3, $4, '{}'::jsonb)`,
					uuid.New(), shelterB, templateOfA, version)

				return err
			})
	}

	taken := attach(1)  // A already has version 1 of this template
	free := attach(742) // A does not

	if taken == nil || free == nil {
		t.Fatal("tenant B attached a version to tenant A's template. The composite key to " +
			"form_templates (id, shelter_id) is what makes the pair unresolvable; " +
			"referential checks bypass row security, so a single-column reference would " +
			"resolve A's template on B's behalf (D5)")
	}

	takenCode := sqlstateOf(t, taken, "the taken version number")
	freeCode := sqlstateOf(t, free, "the free version number")

	if takenCode != freeCode {
		t.Errorf("form_template_versions answers %s for a version number tenant A has "+
			"already used and %s for one it has not. Tenant B can enumerate another "+
			"shelter's form history one number at a time — and uniqueness is checked "+
			"before the foreign key, so a global UNIQUE (template_id, version) is what "+
			"makes the two distinguishable", takenCode, freeCode)
	}
	if takenCode != sqlstateForeignKeyViolation {
		t.Errorf("both attempts were refused with %s rather than a foreign key violation "+
			"(%s). A %s here would mean a unique index answered first, which is the leak "+
			"itself", takenCode, sqlstateForeignKeyViolation, sqlstateUniqueViolation)
	}
}

// Within one shelter the version number still has to be unique, or "publish
// creates version + 1" is a convention rather than a rule and two rows claim to
// be the same version of the same form.
func TestFormTemplateVersions_VersionIsUniqueWithinATemplate(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	shelter := freshTenant(t, env)
	template := seedTemplate(t, env, shelter, "adoption")
	seedVersion(t, env, shelter, template, 1, true)

	// Anti-vacuity: version 2 of the same template is fine, and version 1 of a
	// DIFFERENT template is fine. Without these, a key that refused everything
	// would pass the assertion below.
	seedVersion(t, env, shelter, template, 2, false)
	seedVersion(t, env, shelter, seedTemplate(t, env, shelter, "home_visit"), 1, false)

	err := db.WithTenant(ctx, env.TenantPool, shelter,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO form_template_versions
				     (id, shelter_id, template_id, version, definition)
				 VALUES ($1, $2, $3, 1, '{}'::jsonb)`,
				uuid.New(), shelter, template)

			return err
		})
	if err == nil {
		t.Fatal("the same template now has two rows claiming to be version 1. A submission " +
			"records the version it was filled against, so which definition renders it " +
			"becomes a matter of planner order")
	}
	assertRefusedWith(t, err, sqlstateUniqueViolation, "the duplicate version",
		"UNIQUE (shelter_id, template_id, version) is what refuses it")
}

// sqlstateOf extracts the SQLSTATE from an error that must be a PostgreSQL one.
func sqlstateOf(t *testing.T, err error, what string) string {
	t.Helper()

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("%s failed with a non-PostgreSQL error, so its SQLSTATE cannot be "+
			"compared: %v", what, err)
	}

	return pgErr.Code
}

// Immutability on the child is theatre if the parent's foreign key cascades —
// the lesson T-01-020 paid for on `pet_status_history`, and the same shape here.
//
// `app_tenant` holds DELETE on `form_templates`. With `ON DELETE CASCADE` a
// shelter would erase a published version, and every submission's readability
// with it, by deleting the template — never touching the frozen row, never
// tripping the trigger, straight through the door nobody was watching.
//
// `ON DELETE RESTRICT` closes it, and it is not a workaround: retiring a form is
// what `is_active` is for, so a template with published history is a template
// that stops being offered rather than one that disappears.
func TestPublishedFormVersion_CannotBeErasedByDeletingItsTemplate(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	shelter := freshTenant(t, env)

	deleteTemplate := func(template uuid.UUID) error {
		return db.WithTenant(ctx, env.TenantPool, shelter,
			func(ctx context.Context, tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `DELETE FROM form_templates WHERE id = $1`, template)

				return err
			})
	}

	// Anti-vacuity: a template with no versions CAN still be deleted, so the
	// refusal below belongs to the version and is not a blanket ban on deleting
	// templates — which would pass this test while breaking the product.
	if err := deleteTemplate(seedTemplate(t, env, shelter, "unused")); err != nil {
		t.Fatalf("a template with no versions could not be deleted at all, so the case "+
			"below would prove nothing about the version: %v", err)
	}

	withVersion := seedTemplate(t, env, shelter, "adoption")
	seedVersion(t, env, shelter, withVersion, 1, true)

	err := deleteTemplate(withVersion)
	if err == nil {
		t.Fatal("a template carrying a PUBLISHED version was deleted, and the version went " +
			"with it. Immutability is worth nothing while the parent key cascades: the " +
			"shelter erases the form's history by erasing the form")
	}
	assertRefusedWith(t, err, sqlstateForeignKeyViolation, "deleting the template",
		"ON DELETE RESTRICT on form_template_versions' composite key is what refuses it")
}

// definitionWith builds a template definition carrying exactly these field ids,
// in §4.4's shape. The `span` values are what the 12-column grid requires and
// are here so the fixture is a realistic document rather than a stub.
func definitionWith(fieldIDs ...string) string {
	fields := make([]string, 0, len(fieldIDs))
	for _, id := range fieldIDs {
		fields = append(fields, fmt.Sprintf(
			`{"id":%q,"type":"text","label":%q,"span":6,"required":false}`, id, id))
	}

	return fmt.Sprintf(
		`{"schemaVersion":1,"sections":[{"id":"personal","title":"Datos",`+
			`"rows":[{"id":"r1","fields":[%s]}]}]}`, strings.Join(fields, ","))
}

// publishVersion writes one version with an explicit definition and returns its
// id. Separate from seedVersion because these cases care about the document.
func publishVersion(
	t *testing.T, env *dbtest.Env, shelter, template uuid.UUID, version int, definition string,
) uuid.UUID {
	t.Helper()

	id := uuid.New()
	err := db.WithTenant(context.Background(), env.TenantPool, shelter,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO form_template_versions
				     (id, shelter_id, template_id, version, definition, published_at)
				 VALUES ($1, $2, $3, $4, $5::jsonb, now())`,
				id, shelter, template, version, definition)

			return err
		})
	if err != nil {
		t.Fatalf("publishing version %d: %v", version, err)
	}

	return id
}

// storedDefinition reads a version's definition back as its canonical text, so
// two reads can be compared byte for byte. `jsonb` normalises on input — key
// order and whitespace included — so this compares what is STORED rather than
// what was typed, which is the thing that must not move.
func storedDefinition(t *testing.T, env *dbtest.Env, version uuid.UUID) string {
	t.Helper()

	var text string
	if err := env.OwnerPool.QueryRow(context.Background(),
		`SELECT definition::text FROM form_template_versions WHERE id = $1`,
		version).Scan(&text); err != nil {
		t.Fatalf("reading version %s back: %v", version, err)
	}

	return text
}

// The spec's fourth immutability scenario, and the only one T-01-023 did not
// reach: *"WHEN a changed definition is published for T, THEN a new row exists
// with version = 2, AND version 1 is byte-identical to before, AND
// UNIQUE (template_id, version) refuses a second row with version = 2."*
//
// The three assertions are one property seen from three sides, and each is a
// different bug. A schema that only appended would satisfy the first; one that
// edited in place would satisfy the first and fail the second silently; one with
// no version key would satisfy both and let two rows claim to be version 2.
//
// This is HALF of the historical-readability requirement — the half that lives
// in the versions table. `has_other_pets` disappears from v2 and is still in v1,
// so a renderer pointed at v1 can still resolve it. What is NOT asserted here is
// that a recorded ANSWER keyed `has_other_pets` resolves against v1, because
// that needs `form_submissions` (T-01-025). The spec scenario is written from
// the submission's side, and asserting it from this side only would be implying
// it rather than showing it.
func TestPublishingAVersion_AppendsAndLeavesTheOldOneByteIdentical(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	shelter := freshTenant(t, env)
	template := seedTemplate(t, env, shelter, "adoption")

	v1 := publishVersion(t, env, shelter, template, 1,
		definitionWith("full_name", "email", "has_other_pets"))
	before := storedDefinition(t, env, v1)

	// Anti-vacuity: the field this scenario is about is actually in v1. Without
	// this, a definition that never carried `has_other_pets` would pass the
	// survival check below by having nothing to lose.
	if !strings.Contains(before, "has_other_pets") {
		t.Fatalf("v1 does not contain has_other_pets to begin with, so its survival proves "+
			"nothing: %s", before)
	}

	// The edit: publish a CHANGED definition, which must arrive as a new row.
	v2 := publishVersion(t, env, shelter, template, 2,
		definitionWith("full_name", "email"))

	t.Run("the new version is a new row", func(t *testing.T) {
		var version int
		if err := env.OwnerPool.QueryRow(ctx,
			`SELECT version FROM form_template_versions WHERE id = $1`, v2).Scan(&version); err != nil {
			t.Fatalf("reading v2 back: %v", err)
		}
		if version != 2 {
			t.Errorf("the published row is version %d, want 2", version)
		}
		if strings.Contains(storedDefinition(t, env, v2), "has_other_pets") {
			t.Error("v2 still carries has_other_pets, so this fixture never removed the " +
				"field and the case below is not testing a removal at all")
		}
	})

	t.Run("version 1 is byte-identical to before", func(t *testing.T) {
		after := storedDefinition(t, env, v1)
		if after != before {
			t.Errorf("v1's definition changed when v2 was published.\n before: %s\n  after: %s",
				before, after)
		}
		// Said explicitly as well as by equality, because this is the sentence
		// the whole requirement exists for: a submission recorded against v1
		// still finds its field.
		if !strings.Contains(after, "has_other_pets") {
			t.Error("has_other_pets is gone from v1. A submission recorded against v1 " +
				"answered that field, and it can no longer be rendered")
		}
	})

	t.Run("a second row cannot claim version 2", func(t *testing.T) {
		err := db.WithTenant(ctx, env.TenantPool, shelter,
			func(ctx context.Context, tx pgx.Tx) error {
				_, err := tx.Exec(ctx,
					`INSERT INTO form_template_versions
					     (id, shelter_id, template_id, version, definition)
					 VALUES ($1, $2, $3, 2, '{}'::jsonb)`,
					uuid.New(), shelter, template)

				return err
			})
		if err == nil {
			t.Fatal("two rows now claim to be version 2 of the same template. A submission " +
				"records the version NUMBER it was filled against, so which definition " +
				"renders it becomes a matter of planner order")
		}
		assertRefusedWith(t, err, sqlstateUniqueViolation, "the second version 2",
			"UNIQUE (shelter_id, template_id, version) is what refuses it")
	})
}
