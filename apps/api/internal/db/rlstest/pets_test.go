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
)

// seedPet creates one pet for a shelter, as that tenant, and returns its id.
// Columns beyond the required ones are left to the migration's defaults, which
// is itself worth exercising: `status` must default to 'draft', so a pet is not
// published by forgetting to say so.
func seedPet(t *testing.T, env *dbtest.Env, shelter uuid.UUID) uuid.UUID {
	t.Helper()

	id := uuid.New()
	err := db.WithTenant(context.Background(), env.TenantPool, shelter,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO pets (id, shelter_id, public_code, name, species_id, size)
				 VALUES ($1, $2, $3, 'Firulais',
				         (SELECT id FROM species WHERE code = 'dog'), 'm')`,
				id, shelter, "P-"+uuid.NewString())

			return err
		})
	if err != nil {
		t.Fatalf("seeding a pet for %s: %v", shelter, err)
	}

	return id
}

// publish moves a pet into the state the public catalog admits.
func publish(t *testing.T, env *dbtest.Env, shelter, pet uuid.UUID) {
	t.Helper()

	err := db.WithTenant(context.Background(), env.TenantPool, shelter,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`UPDATE pets SET status = 'available', published_at = now() WHERE id = $1`, pet)

			return err
		})
	if err != nil {
		t.Fatalf("publishing pet %s: %v", pet, err)
	}
}

// publiclyVisible reports whether app_public can see this pet, read through
// WithPublic: a separate role, a separate pool, a read-only transaction and no
// tenant scope at all.
func publiclyVisible(t *testing.T, env *dbtest.Env, pet uuid.UUID) bool {
	t.Helper()

	var found int
	err := db.WithPublic(context.Background(), env.PublicPool,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT count(*) FROM pets WHERE id = $1`, pet).Scan(&found)
		})
	if err != nil {
		t.Fatalf("reading pets as app_public: %v", err)
	}

	return found == 1
}

// The public catalog's three conditions, each retracted on its own.
//
// One test per condition rather than one test with three assertions, because
// the realistic bug is a predicate that drops ONE of the three — an `AND`
// turned into an `OR`, a clause deleted while debugging — and a single case
// that flips all three at once cannot tell which.
//
// The default state matters as much as the transitions: a freshly created pet
// is `draft` with a null `published_at`, so **forgetting to publish keeps it
// private**. A default of 'available' would make the safe path the one nobody
// takes.
func TestPublicCatalog_ShowsOnlyPublishedAvailableUndeletedPets(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	if err := seedShelters(ctx, env.OwnerPool, env.ShelterA); err != nil {
		t.Fatalf("seeding shelters: %v", err)
	}

	t.Run("a new pet is private until it is published", func(t *testing.T) {
		pet := seedPet(t, env, env.ShelterA)
		if publiclyVisible(t, env, pet) {
			t.Error("a pet was public the moment it was created. `status` must default to " +
				"'draft' and `published_at` to NULL, so publishing is something a shelter " +
				"does on purpose rather than something it forgets to prevent")
		}
	})

	t.Run("a published available pet is public", func(t *testing.T) {
		pet := seedPet(t, env, env.ShelterA)
		publish(t, env, env.ShelterA, pet)
		if !publiclyVisible(t, env, pet) {
			t.Error("a published, available, undeleted pet is invisible to the public " +
				"catalog, so every negative case below would pass for the wrong reason")
		}
	})

	for _, tc := range []struct {
		name    string
		retract string
		why     string
	}{
		{
			name:    "clearing published_at retracts it",
			retract: `UPDATE pets SET published_at = NULL WHERE id = $1`,
			why:     "a shelter must be able to pull a listing without destroying the record",
		},
		{
			name:    "leaving 'available' retracts it",
			retract: `UPDATE pets SET status = 'reserved' WHERE id = $1`,
			why: "a reserved animal is still in the shelter's own lists and must not be " +
				"in the public one",
		},
		{
			name:    "logical deletion retracts it",
			retract: `UPDATE pets SET deleted_at = now() WHERE id = $1`,
			why: "logical deletion is the model's rule; a deleted row that stayed public " +
				"would make deletion cosmetic",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pet := seedPet(t, env, env.ShelterA)
			publish(t, env, env.ShelterA, pet)
			if !publiclyVisible(t, env, pet) {
				t.Fatal("the pet was not public before the retraction, so this proves nothing")
			}

			err := db.WithTenant(ctx, env.TenantPool, env.ShelterA,
				func(ctx context.Context, tx pgx.Tx) error {
					_, err := tx.Exec(ctx, tc.retract, pet)

					return err
				})
			if err != nil {
				t.Fatalf("retracting: %v", err)
			}

			if publiclyVisible(t, env, pet) {
				t.Errorf("the pet is still in the public catalog. %s", tc.why)
			}
		})
	}
}

// The public catalog spans shelters, and that is the product requirement rather
// than a leak: app_public never sets app.shelter_id, so its policy reads no
// tenant scope at all.
//
// Asserted because the natural mistake is to reuse the tenant template here,
// which would make the public site show whichever shelter happened to be in
// context — or, with no scope set, nothing at all.
func TestPublicCatalog_SpansShelters(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	if err := seedShelters(ctx, env.OwnerPool, env.ShelterA, env.ShelterB); err != nil {
		t.Fatalf("seeding shelters: %v", err)
	}

	petOfA := seedPet(t, env, env.ShelterA)
	petOfB := seedPet(t, env, env.ShelterB)
	publish(t, env, env.ShelterA, petOfA)
	publish(t, env, env.ShelterB, petOfB)

	for name, pet := range map[string]uuid.UUID{"shelter A's pet": petOfA, "shelter B's pet": petOfB} {
		if !publiclyVisible(t, env, pet) {
			t.Errorf("%s is not in the public catalog. The public site is one catalog "+
				"across every verified shelter, not one per tenant", name)
		}
	}
}

// §4.3's closed unions, and the spec's own scenario: an out-of-domain value is
// rejected by the database, not by whoever remembered to validate.
//
// Every constrained column is covered rather than a representative one, because
// the failure is per-column: a CHECK omitted from `size` is invisible to a test
// that only exercises `status`.
func TestPets_ConstrainedColumnsRejectOutOfDomainValues(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	if err := seedShelters(ctx, env.OwnerPool, env.ShelterA); err != nil {
		t.Fatalf("seeding shelters: %v", err)
	}
	pet := seedPet(t, env, env.ShelterA)

	const sqlstateCheckViolation = "23514"

	for _, tc := range []struct {
		column string
		valid  string
		bogus  string
	}{
		{"sex", "female", "other"},
		{"size", "xl", "enormous"},
		{"age_precision", "year", "roughly"},
		{"status", "adopted", "rehomed"},
		{"energy_level", "high", "extreme"},
		{"good_with_kids", "no", "sometimes"},
		{"good_with_dogs", "yes", "maybe"},
		{"good_with_cats", "unknown", "depends"},
	} {
		t.Run(tc.column, func(t *testing.T) {
			set := func(value string) error {
				return db.WithTenant(ctx, env.TenantPool, env.ShelterA,
					func(ctx context.Context, tx pgx.Tx) error {
						_, err := tx.Exec(ctx,
							`UPDATE pets SET `+tc.column+` = $1 WHERE id = $2`, value, pet)

						return err
					})
			}

			// Anti-vacuity: a declared value must be accepted, or a column
			// rejecting everything would pass the assertion below.
			if err := set(tc.valid); err != nil {
				t.Fatalf("%s rejected the declared value %q: %v", tc.column, tc.valid, err)
			}

			err := set(tc.bogus)
			if err == nil {
				t.Fatalf("%s accepted %q. §4.3 makes it a closed union, and the adopter "+
					"filter is built on these values", tc.column, tc.bogus)
			}

			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != sqlstateCheckViolation {
				t.Fatalf("%s refused %q, but not with a check violation (%s): %v",
					tc.column, tc.bogus, sqlstateCheckViolation, err)
			}
		})
	}
}

// §4.6's two indexes, and the spec's scenario asks for them by shape.
//
// They are read from the catalog rather than trusted, because an index is the
// one thing in a migration whose absence changes nothing observable until the
// table is large — at which point the catalog is already slow in production and
// nobody remembers the index was ever specified.
func TestPets_HasTheFilterIndexes(t *testing.T) {
	env := dbtest.Postgres(t)

	rows, err := env.OwnerPool.Query(context.Background(), `
		SELECT indexdef FROM pg_indexes
		WHERE schemaname = 'public' AND tablename = 'pets'`)
	if err != nil {
		t.Fatalf("reading pets' indexes: %v", err)
	}
	defer rows.Close()

	var defs []string
	for rows.Next() {
		var def string
		if err := rows.Scan(&def); err != nil {
			t.Fatalf("scanning an index: %v", err)
		}
		defs = append(defs, def)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading pets' indexes: %v", err)
	}

	for _, want := range []struct {
		what     string
		fragment []string
		why      string
	}{
		{
			what:     "the shelter dashboard index",
			fragment: []string{"shelter_id", "status", "published_at DESC"},
			why:      "§4.6 — every shelter-side listing orders by publication within a status",
		},
		{
			what:     "the adopter filter index",
			fragment: []string{"species_id", "size", "energy_level", "WHERE", "'available'"},
			why: "§4.6 — partial on available pets, because the public catalog never reads " +
				"anything else and a partial index is a fraction of the size",
		},
	} {
		if !anyIndexContains(defs, want.fragment) {
			t.Errorf("%s is missing. %s\ngot:\n  %v", want.what, want.why, defs)
		}
	}
}

func anyIndexContains(defs []string, fragments []string) bool {
	for _, def := range defs {
		lowered := strings.ToLower(def)
		matched := true
		for _, fragment := range fragments {
			if !strings.Contains(lowered, strings.ToLower(fragment)) {
				matched = false

				break
			}
		}
		if matched {
			return true
		}
	}

	return false
}

// `media`'s public policy has now been scheduled against a dependency it does
// not have TWICE: design §Public surface put it in 00003, and T-01-017 moved it
// to 00005 believing it depended on `pets`. It depends on `pet_media` — the
// spec's own scenario is "media M is ATTACHED to a draft pet" — which is 00006.
//
// This is the guard, in both directions, so the deferral cannot survive a third
// time by being forgotten.
func TestMedia_GetsItsPublicPolicyWhenPetMediaLands(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	var attachmentsExist bool
	if err := env.OwnerPool.QueryRow(ctx,
		`SELECT to_regclass('public.pet_media') IS NOT NULL`).Scan(&attachmentsExist); err != nil {
		t.Fatalf("looking for pet_media: %v", err)
	}

	var publicPolicies int
	if err := env.OwnerPool.QueryRow(ctx, `
		SELECT count(*)
		FROM pg_policy p
		JOIN pg_class c ON c.oid = p.polrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relname = 'media'
		  AND 'app_public'::regrole = ANY (p.polroles)`).Scan(&publicPolicies); err != nil {
		t.Fatalf("counting media's public policies: %v", err)
	}

	switch {
	case attachmentsExist && publicPolicies == 0:
		t.Error("`pet_media` exists but `media` still has no app_public policy. The public " +
			"catalog cannot show a photo, and the deferral that started in 00003 has now " +
			"outlived two migrations that were each supposed to end it")
	case !attachmentsExist && publicPolicies > 0:
		t.Error("`media` carries an app_public policy but `pet_media` does not exist, so " +
			"whatever that policy scopes on, it is not attachment to a published pet")
	}

	// While it is deferred, the safe direction is closed: no policy AND no
	// grant, so app_public sees no media at all rather than all of it.
	if !attachmentsExist {
		var granted bool
		if err := env.OwnerPool.QueryRow(ctx,
			`SELECT has_table_privilege('app_public', 'media', 'SELECT')`).Scan(&granted); err != nil {
			t.Fatalf("reading app_public's privilege on media: %v", err)
		}
		if granted {
			t.Error("app_public holds SELECT on media while the policy that would narrow " +
				"it does not exist yet. A grant without its policy is the widest possible " +
				"state, not the safest")
		}
	}
}
