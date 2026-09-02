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
)

// sqlstateForeignKeyViolation is what a composite tenant key raises when a child
// points across tenants.
const sqlstateForeignKeyViolation = "23503"

// The first composite tenant foreign key in this schema, and the assertion D5
// exists for.
//
// Referential integrity checks ALWAYS bypass row security — PostgreSQL runs them
// as the constraint's owner, not as the querying role. So a plain
// `logo_media_id uuid REFERENCES media (id)` would let shelter B set its logo to
// a media row owned by shelter A: a row B cannot see, cannot read, cannot list,
// and would nonetheless be publishing on its own page. The policy does not stop
// it, because the policy is never consulted.
//
// `FOREIGN KEY (logo_media_id, id) REFERENCES media (id, shelter_id)` makes that
// unrepresentable rather than merely forbidden. This test is what proves the
// difference is real, because both spellings compile, both migrate, and only one
// of them is correct.
func TestShelters_CannotClaimAnotherTenantsMedia(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	if err := seedShelters(ctx, env.OwnerPool, env.ShelterA, env.ShelterB); err != nil {
		t.Fatalf("seeding shelters: %v", err)
	}

	// A media row owned by A, created by A.
	mediaOfA := uuid.New()
	if err := db.WithTenant(ctx, env.TenantPool, env.ShelterA,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO media (id, shelter_id, kind, storage_key, mime, bytes)
				 VALUES ($1, $2, 'image', $3, 'image/avif', 2048)`,
				mediaOfA, env.ShelterA, "shelters/a/"+mediaOfA.String())

			return err
		}); err != nil {
		t.Fatalf("seeding tenant A's media row: %v", err)
	}

	// Anti-vacuity: A can point at its OWN media row. Without this, a
	// constraint that refused every value whatsoever would pass below.
	if err := db.WithTenant(ctx, env.TenantPool, env.ShelterA,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`UPDATE shelters SET logo_media_id = $1 WHERE id = $2`, mediaOfA, env.ShelterA)

			return err
		}); err != nil {
		t.Fatalf("tenant A could not set its logo to its own media row, so the constraint "+
			"is refusing legitimate use and the cross-tenant case below proves nothing: %v",
			err)
	}

	for _, column := range []string{"logo_media_id", "cover_media_id"} {
		t.Run(column, func(t *testing.T) {
			err := db.WithTenant(ctx, env.TenantPool, env.ShelterB,
				func(ctx context.Context, tx pgx.Tx) error {
					_, err := tx.Exec(ctx,
						`UPDATE shelters SET `+column+` = $1 WHERE id = $2`,
						mediaOfA, env.ShelterB)

					return err
				})
			if err == nil {
				t.Fatalf("tenant B set its %s to a media row owned by tenant A. B cannot "+
					"see that row, cannot read it, and would be publishing it: the "+
					"foreign key is single-column and referential checks bypass row "+
					"security (D5)", column)
			}

			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != sqlstateForeignKeyViolation {
				t.Fatalf("the cross-tenant claim failed, but not with a foreign key "+
					"violation (%s), so what refused it is unproven: %v",
					sqlstateForeignKeyViolation, err)
			}
		})
	}
}

// Every composite tenant key, declared by its own parent.
//
// The key is only as good as the UNIQUE constraint it references, and that
// constraint is easy to lose: it looks redundant next to the primary key —
// redundant because `id` is already unique — so "tidying" it away is a plausible
// later edit that no other test would catch. Mutation confirmed exactly that on
// 2026-08-30: dropping it from `pets` broke nothing.
//
// Every migration that creates a composite-FK parent adds its row here, the same
// way it adds rows to the policy and grant inventory.
func TestCompositeTenantKeys_AreDeclaredByTheirParents(t *testing.T) {
	env := dbtest.Postgres(t)

	for _, tc := range []struct {
		table   string
		columns []string
		needed  string
	}{
		{"media", []string{"id", "shelter_id"}, "shelters' logo and cover keys today, " +
			"documents' composite key at T-01-031"},
		{"pets", []string{"id", "shelter_id"}, "pet_media, pet_health_records and " +
			"pet_status_history at T-01-020"},
		{"form_templates", []string{"id", "shelter_id"},
			"form_template_versions at T-01-023"},
		{"form_template_versions", []string{"id", "shelter_id"},
			"form_submissions at T-01-025 — declared by 00008 rather than 00007, " +
				"because 00007 had already run"},
		{"adoption_applications", []string{"id", "shelter_id"},
			"application_events and application_notes at T-01-029, documents at T-01-031"},
	} {
		t.Run(tc.table, func(t *testing.T) {
			if !hasUniqueOn(t, env, tc.table, tc.columns) {
				t.Errorf("%s has no UNIQUE %v. It looks redundant beside the primary key "+
					"and is not: it is the REFERENCED key of every composite tenant "+
					"foreign key pointing at %s (D5) — needed by %s",
					tc.table, tc.columns, tc.table, tc.needed)
			}
		})
	}
}

// A microchip number is globally unique in the world, so `UNIQUE (microchip_id)`
// is the "correct" modelling — and it is a cross-tenant information leak.
//
// Uniqueness violations are raised before any policy is consulted (proven in
// T-01-013 for the forged-insert path), so a global constraint turns every
// INSERT into an existence oracle: tenant A types a chip number, gets 23505, and
// has learned that some other shelter holds that animal. Scoping the key per
// shelter keeps the constraint useful and the oracle shut.
//
// Cross-shelter duplicate detection is a job for a moderation view under the
// owner role, not for a constraint every tenant can probe.
func TestPets_MicrochipUniquenessIsPerShelterNotGlobal(t *testing.T) {
	env := dbtest.Postgres(t)

	if !hasUniqueOn(t, env, "pets", []string{"shelter_id", "microchip_id"}) {
		t.Error("pets has no UNIQUE (shelter_id, microchip_id), so one shelter can register " +
			"the same chip twice")
	}
	if hasUniqueOn(t, env, "pets", []string{"microchip_id"}) {
		t.Error("pets has a GLOBAL UNIQUE (microchip_id). Uniqueness is checked before any " +
			"policy, so every INSERT becomes an existence oracle over other shelters' " +
			"animals: type a chip number, read the 23505, learn who else holds it")
	}
}

// hasUniqueOn reports whether the table carries a unique constraint on exactly
// these columns, in this order.
func hasUniqueOn(t *testing.T, env *dbtest.Env, table string, columns []string) bool {
	t.Helper()

	var present bool
	err := env.OwnerPool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1
			FROM pg_constraint c
			WHERE c.conrelid = ('public.' || $1)::regclass
			  AND c.contype = 'u'
			  AND c.conkey = (
			        SELECT array_agg(a.attnum ORDER BY ordinality)
			        FROM unnest($2::text[]) WITH ORDINALITY AS want(name, ordinality)
			        JOIN pg_attribute a
			          ON a.attrelid = c.conrelid AND a.attname = want.name
			      )
		)`, table, columns).Scan(&present)
	if err != nil {
		t.Fatalf("reading %s's unique constraints: %v", table, err)
	}

	return present
}

// `kind` is a CLOSED union — §4.2 gives exactly `image` and `document` — and the
// closure is enforced by the database rather than by whoever writes the insert.
//
// It matters past tidiness: Phase 04's derivation pipeline branches on `kind`,
// so a third value would reach a switch with no arm for it. Mutation found this
// unasserted on 2026-08-30: dropping the CHECK broke nothing, which is how a
// constraint quietly stops existing.
func TestMedia_KindIsAClosedUnion(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	if err := seedShelters(ctx, env.OwnerPool, env.ShelterA); err != nil {
		t.Fatalf("seeding shelters: %v", err)
	}

	insert := func(kind string) error {
		id := uuid.New()

		return db.WithTenant(ctx, env.TenantPool, env.ShelterA,
			func(ctx context.Context, tx pgx.Tx) error {
				_, err := tx.Exec(ctx,
					`INSERT INTO media (id, shelter_id, kind, storage_key, mime, bytes)
					 VALUES ($1, $2, $3, $4, 'application/octet-stream', 1)`,
					id, env.ShelterA, kind, "shelters/kind/"+id.String())

				return err
			})
	}

	// Anti-vacuity: both declared members are accepted, so a constraint that
	// refused everything would not pass as a closed union.
	for _, kind := range []string{"image", "document"} {
		if err := insert(kind); err != nil {
			t.Fatalf("kind %q is declared by §4.2 and was refused: %v", kind, err)
		}
	}

	const sqlstateCheckViolation = "23514"

	err := insert("video")
	if err == nil {
		t.Fatal("a media row with kind='video' was accepted. The union is not closed, and " +
			"Phase 04's derivation pipeline branches on this column")
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != sqlstateCheckViolation {
		t.Fatalf("the undeclared kind was refused, but not by a check constraint (%s): %v",
			sqlstateCheckViolation, err)
	}
}

// Logical deletion is the model's rule for every table carrying `deleted_at`
// (§4.2): rows are not physically removed by application paths.
//
// What this asserts is narrow and deliberate — that the column exists, is
// nullable, and defaults to NULL, so a freshly inserted row is live. The
// behavioural half, that application paths soft-delete instead of DELETE, has no
// application paths to test yet; Phase 04 owns it.
func TestMedia_DeletedAtIsNullableAndDefaultsToLive(t *testing.T) {
	env := dbtest.Postgres(t)

	var notNull, hasDefault bool
	err := env.OwnerPool.QueryRow(context.Background(), `
		SELECT a.attnotnull, d.adbin IS NOT NULL
		FROM pg_attribute a
		LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
		WHERE a.attrelid = 'public.media'::regclass AND a.attname = 'deleted_at'`).
		Scan(&notNull, &hasDefault)
	if err != nil {
		t.Fatalf("reading media.deleted_at: %v", err)
	}
	if notNull {
		t.Error("media.deleted_at is NOT NULL, so no row can be live")
	}
	if hasDefault {
		t.Error("media.deleted_at has a default, so rows would arrive already deleted")
	}
}
