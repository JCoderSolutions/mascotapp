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

// sqlstateRaiseException is what a plpgsql RAISE EXCEPTION reports by default.
// It is deliberately NOT 42501: the grant and the trigger are two different
// layers, and a test that could not tell them apart would keep passing with one
// of them deleted.
const sqlstateRaiseException = "P0001"

// seedHistory appends one status change to a pet, as the given tenant, and
// returns the row's id.
func seedHistory(t *testing.T, env *dbtest.Env, shelter, pet uuid.UUID) uuid.UUID {
	t.Helper()

	id := uuid.New()
	err := db.WithTenant(context.Background(), env.TenantPool, shelter,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO pet_status_history
				     (id, shelter_id, pet_id, from_status, to_status, reason)
				 VALUES ($1, $2, $3, 'draft', 'available', 'ready')`,
				id, shelter, pet)

			return err
		})
	if err != nil {
		t.Fatalf("appending a status change for pet %s: %v", pet, err)
	}

	return id
}

// The grant is what stops `app_tenant`. This is what stops everyone else.
//
// LT-5 makes `pet_status_history` immutable from day one, and the revoked
// UPDATE/DELETE grant plus the absent UPDATE/DELETE policies deliver that for
// the application role. Neither survives a role with BYPASSRLS — and on Neon
// that is not hypothetical: `neon_superuser` carries it. RLS is bypassed by such
// roles; TRIGGERS ARE NOT.
//
// So this test runs as the OWNER, which in this container is a superuser: the
// role that holds every privilege and skips every policy. If the trail is still
// immutable to it, the trigger is doing the work, because nothing else here
// could be.
//
// It also converts a silent zero-row result — which application code readily
// misreads as success — into a loud error.
func TestPetStatusHistory_IsAppendOnlyEvenForARoleThatBypassesRowSecurity(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	if err := seedShelters(ctx, env.OwnerPool, env.ShelterA); err != nil {
		t.Fatalf("seeding shelters: %v", err)
	}
	pet := seedPet(t, env, env.ShelterA)
	history := seedHistory(t, env, env.ShelterA, pet)

	// Anti-vacuity: the owner CAN append. Without this, a table nobody could
	// write to at all would pass both cases below and read as append-only.
	if _, err := env.OwnerPool.Exec(ctx,
		`INSERT INTO pet_status_history (id, shelter_id, pet_id, from_status, to_status)
		 VALUES ($1, $2, $3, 'available', 'adopted')`,
		uuid.New(), env.ShelterA, pet); err != nil {
		t.Fatalf("the owner cannot APPEND to pet_status_history, so the refusals below "+
			"would prove the table is unusable rather than append-only: %v", err)
	}

	for _, tc := range []struct {
		name      string
		statement string
	}{
		{"update", `UPDATE pet_status_history SET reason = 'rewritten' WHERE id = $1`},
		{"delete", `DELETE FROM pet_status_history WHERE id = $1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := env.OwnerPool.Exec(ctx, tc.statement, history)
			if err == nil {
				t.Fatalf("a superuser %sd a row of pet_status_history. The grant and the "+
					"missing policy both stop app_tenant and neither stops a BYPASSRLS "+
					"role; only a trigger does, and there is none", tc.name)
			}

			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != sqlstateRaiseException {
				t.Fatalf("the %s was refused, but not by the append-only trigger (%s), so "+
					"what refused it is unproven: %v", tc.name, sqlstateRaiseException, err)
			}
		})
	}

	// TRUNCATE is the one write no row-level policy can ever see: it removes
	// every row without visiting any, so USING and WITH CHECK are never
	// consulted. Only a statement-level trigger reaches it.
	t.Run("truncate", func(t *testing.T) {
		_, err := env.OwnerPool.Exec(ctx, `TRUNCATE pet_status_history`)
		if err == nil {
			t.Fatal("pet_status_history was TRUNCATEd. A row-level policy cannot see a " +
				"truncate at all, so the BEFORE TRUNCATE statement trigger is the only " +
				"thing that could have stopped it")
		}

		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != sqlstateRaiseException {
			t.Fatalf("the truncate was refused, but not by the append-only trigger (%s): %v",
				sqlstateRaiseException, err)
		}
	})
}

// Append-only on the child is theatre if the parent's foreign key cascades.
//
// `app_tenant` holds DELETE on `pets`. With `ON DELETE CASCADE` on the history's
// composite key, a shelter would erase an animal's entire trail by deleting the
// animal — never touching the protected table, never tripping the trigger, and
// defeating LT-5 through the one door nobody was watching.
//
// `ON DELETE RESTRICT` closes it, and it is not a workaround bolted onto the
// side: LT-5's own rule is that a pet leaves the catalog by CHANGING STATUS,
// never by disappearing. A pet with a recorded history is therefore a pet that
// can only be soft-deleted, which is what the product wanted in the first place.
func TestPetStatusHistory_CannotBeErasedByDeletingItsPet(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	if err := seedShelters(ctx, env.OwnerPool, env.ShelterA); err != nil {
		t.Fatalf("seeding shelters: %v", err)
	}

	deletePet := func(pet uuid.UUID) error {
		return db.WithTenant(ctx, env.TenantPool, env.ShelterA,
			func(ctx context.Context, tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `DELETE FROM pets WHERE id = $1`, pet)

				return err
			})
	}

	// Anti-vacuity: a pet with no history CAN still be hard-deleted, so the
	// refusal below is the history's doing and not a blanket ban on deleting
	// pets — which would pass this test while breaking the product.
	if err := deletePet(seedPet(t, env, env.ShelterA)); err != nil {
		t.Fatalf("a pet with no status history could not be deleted at all, so the case "+
			"below would prove nothing about the history: %v", err)
	}

	withHistory := seedPet(t, env, env.ShelterA)
	seedHistory(t, env, env.ShelterA, withHistory)

	err := deletePet(withHistory)
	if err == nil {
		t.Fatal("a pet carrying status history was hard-deleted, and its trail went with " +
			"it. Append-only on pet_status_history is worth nothing while its foreign key " +
			"cascades: the shelter erases the record by erasing the animal")
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != sqlstateForeignKeyViolation {
		t.Fatalf("the delete was refused, but not by the restricting foreign key (%s): %v",
			sqlstateForeignKeyViolation, err)
	}
}

// `pet_health_records.document_media_id` points at `media`, and D5 applies to it
// exactly as it applies to the parent key.
//
// This is the second reference on the same row, and the easy one to miss: the
// composite key to `pets` is the one the design writes out, so a single-column
// `REFERENCES media (id)` here would look finished. It is the same hole
// T-01-017 closed on `shelters.logo_media_id` — a tenant attaching another
// shelter's document to its own record, through a reference whose check bypasses
// row security.
func TestPetHealthRecords_CannotAttachAnotherTenantsDocument(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	if err := seedShelters(ctx, env.OwnerPool, env.ShelterA, env.ShelterB); err != nil {
		t.Fatalf("seeding shelters: %v", err)
	}

	documentOfA := uuid.New()
	if err := db.WithTenant(ctx, env.TenantPool, env.ShelterA,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO media (id, shelter_id, kind, storage_key, mime, bytes)
				 VALUES ($1, $2, 'document', $3, 'application/pdf', 4096)`,
				documentOfA, env.ShelterA, "shelters/a/"+documentOfA.String())

			return err
		}); err != nil {
		t.Fatalf("seeding tenant A's document: %v", err)
	}

	petOfB := seedPet(t, env, env.ShelterB)

	// Anti-vacuity: B can attach its OWN document. A constraint refusing every
	// value would otherwise pass the cross-tenant case below.
	documentOfB := uuid.New()
	if err := db.WithTenant(ctx, env.TenantPool, env.ShelterB,
		func(ctx context.Context, tx pgx.Tx) error {
			if _, err := tx.Exec(ctx,
				`INSERT INTO media (id, shelter_id, kind, storage_key, mime, bytes)
				 VALUES ($1, $2, 'document', $3, 'application/pdf', 4096)`,
				documentOfB, env.ShelterB, "shelters/b/"+documentOfB.String()); err != nil {
				return err
			}
			_, err := tx.Exec(ctx,
				`INSERT INTO pet_health_records
				     (id, shelter_id, pet_id, type, occurred_on, document_media_id)
				 VALUES ($1, $2, $3, 'vaccine', current_date, $4)`,
				uuid.New(), env.ShelterB, petOfB, documentOfB)

			return err
		}); err != nil {
		t.Fatalf("tenant B could not attach its own document, so the cross-tenant case "+
			"below proves nothing: %v", err)
	}

	err := db.WithTenant(ctx, env.TenantPool, env.ShelterB,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO pet_health_records
				     (id, shelter_id, pet_id, type, occurred_on, document_media_id)
				 VALUES ($1, $2, $3, 'vaccine', current_date, $4)`,
				uuid.New(), env.ShelterB, petOfB, documentOfA)

			return err
		})
	if err == nil {
		t.Fatal("tenant B attached tenant A's document to its own health record. B cannot " +
			"see that media row and would still be linking it: the reference is " +
			"single-column, and referential checks bypass row security (D5)")
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != sqlstateForeignKeyViolation {
		t.Fatalf("the cross-tenant attachment failed, but not with a foreign key violation "+
			"(%s), so what refused it is unproven: %v", sqlstateForeignKeyViolation, err)
	}
}

// One primary photo per pet — scoped per shelter, and NOT globally, for the same
// reason `pets.microchip_id` is scoped per shelter (T-01-019).
//
// `UNIQUE (pet_id) WHERE is_primary` reads as the natural spelling and is a
// cross-tenant existence oracle: uniqueness is checked before any policy, so
// tenant B could insert an attachment naming tenant A's pet and read the answer
// off the SQLSTATE — 23505 means that pet already has a cover photo, 23503 means
// it does not exist or is not B's. Two different answers to a question B has no
// right to ask.
//
// Adding `shelter_id` to the key keeps the guarantee identical inside a tenant —
// a pet belongs to exactly one shelter, so per-shelter uniqueness over its
// attachments IS per-pet uniqueness — while making the two SQLSTATEs collapse
// into one for everybody else.
func TestPetMedia_PrimaryPhotoUniquenessIsPerShelterNotGlobal(t *testing.T) {
	env := dbtest.Postgres(t)

	if !hasPartialUniqueOn(t, env, "pet_media", []string{"shelter_id", "pet_id"}) {
		t.Error("pet_media has no partial UNIQUE (shelter_id, pet_id) over primary " +
			"attachments, so a pet can carry two cover photos and the card renderer picks " +
			"whichever the planner returns first")
	}
	if hasPartialUniqueOn(t, env, "pet_media", []string{"pet_id"}) {
		t.Error("pet_media has a partial UNIQUE keyed on pet_id alone. Uniqueness is " +
			"checked before any policy, so tenant B can tell 23505 from 23503 and learn " +
			"whether another shelter's pet already has a cover photo")
	}
}

// hasPartialUniqueOn reports whether a PARTIAL unique index over exactly these
// columns exists. It reads pg_index rather than pg_constraint because a partial
// unique index cannot be a table constraint at all — `UNIQUE (...) WHERE ...` is
// not valid constraint syntax — so hasUniqueOn would report false for a
// perfectly good key and the assertion would be unfalsifiable.
func hasPartialUniqueOn(t *testing.T, env *dbtest.Env, table string, columns []string) bool {
	t.Helper()

	// indkey is an int2vector, and casting it to int2[] yields an array whose
	// LOWER BOUND IS 0 — `[0:1]={3,1}`. array_agg builds one based at 1.
	// PostgreSQL's array equality compares bounds as well as elements, so the
	// two never match however right the index is, and the assertion silently
	// becomes unfalsifiable. Re-aggregating indkey through unnest rebases it.
	var present bool
	err := env.OwnerPool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1
			FROM pg_index i
			WHERE i.indrelid = ('public.' || $1)::regclass
			  AND i.indisunique
			  AND i.indpred IS NOT NULL
			  AND (SELECT array_agg(k ORDER BY ord)
			       FROM unnest(i.indkey::int2[]) WITH ORDINALITY AS got(k, ord))
			      = (
			        SELECT array_agg(a.attnum ORDER BY ordinality)
			        FROM unnest($2::text[]) WITH ORDINALITY AS want(name, ordinality)
			        JOIN pg_attribute a
			          ON a.attrelid = i.indrelid AND a.attname = want.name
			      )
		)`, table, columns).Scan(&present)
	if err != nil {
		t.Fatalf("reading %s's partial unique indexes: %v", table, err)
	}

	return present
}

// The spec scenario, and the deferral that took three migrations to land:
// *"media M is attached only to a draft pet — M is not returned to app_public."*
//
// What makes this work is composition, not duplication. `media`'s public policy
// says only "there is an attachment", `pet_media`'s says only "there is a pet",
// and each subquery is itself filtered by the referenced table's RLS under
// app_public — the same mechanism that makes `users` visible only through
// `memberships` (D6). So the definition of "a public pet" lives in exactly one
// place, `public_catalog` on `pets`, and a photo follows its pet automatically.
//
// That subtlety is the reason this test walks the pet through all three states
// instead of checking the policy text: the chain is what has to hold, and only
// behaviour can show it does.
func TestPublicMedia_IsReachableOnlyThroughAPublicPet(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	if err := seedShelters(ctx, env.OwnerPool, env.ShelterA); err != nil {
		t.Fatalf("seeding shelters: %v", err)
	}

	pet := seedPet(t, env, env.ShelterA)
	photo := uuid.New()
	if err := db.WithTenant(ctx, env.TenantPool, env.ShelterA,
		func(ctx context.Context, tx pgx.Tx) error {
			if _, err := tx.Exec(ctx,
				`INSERT INTO media (id, shelter_id, kind, storage_key, mime, bytes)
				 VALUES ($1, $2, 'image', $3, 'image/avif', 2048)`,
				photo, env.ShelterA, "shelters/a/"+photo.String()); err != nil {
				return err
			}
			_, err := tx.Exec(ctx,
				`INSERT INTO pet_media (pet_id, media_id, shelter_id, position, is_primary)
				 VALUES ($1, $2, $3, 0, true)`,
				pet, photo, env.ShelterA)

			return err
		}); err != nil {
		t.Fatalf("attaching a photo to the pet: %v", err)
	}

	if publiclyVisibleMedia(t, env, photo) {
		t.Error("a photo attached to a DRAFT pet is visible to app_public. The pet itself " +
			"is not public, so nothing about it should be")
	}

	publish(t, env, env.ShelterA, pet)
	if !publiclyVisibleMedia(t, env, photo) {
		t.Fatal("the photo of a published, available pet is invisible to app_public, so " +
			"every negative case here would pass for the wrong reason and the public " +
			"catalog would ship with no images")
	}

	// Retracting the PET must retract its photo, without anybody touching the
	// media row. This is what proves the chain composes rather than each policy
	// carrying its own stale copy of what "public" means.
	if err := db.WithTenant(ctx, env.TenantPool, env.ShelterA,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`UPDATE pets SET published_at = NULL WHERE id = $1`, pet)

			return err
		}); err != nil {
		t.Fatalf("retracting the pet: %v", err)
	}
	if publiclyVisibleMedia(t, env, photo) {
		t.Error("the pet was retracted and its photo stayed public. media's policy is not " +
			"reading the pet's state through pet_media; it is deciding on its own, and it " +
			"will drift from public_catalog the first time that predicate changes")
	}
}

// A soft-deleted media row leaves the public catalog even while its pet stays
// published. It is the one condition `media` owns rather than inherits, so
// nothing else in this file can catch it.
func TestPublicMedia_SoftDeletedPhotoLeavesTheCatalog(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	if err := seedShelters(ctx, env.OwnerPool, env.ShelterA); err != nil {
		t.Fatalf("seeding shelters: %v", err)
	}

	pet := seedPet(t, env, env.ShelterA)
	photo := uuid.New()
	if err := db.WithTenant(ctx, env.TenantPool, env.ShelterA,
		func(ctx context.Context, tx pgx.Tx) error {
			if _, err := tx.Exec(ctx,
				`INSERT INTO media (id, shelter_id, kind, storage_key, mime, bytes)
				 VALUES ($1, $2, 'image', $3, 'image/avif', 2048)`,
				photo, env.ShelterA, "shelters/a/"+photo.String()); err != nil {
				return err
			}
			_, err := tx.Exec(ctx,
				`INSERT INTO pet_media (pet_id, media_id, shelter_id, position)
				 VALUES ($1, $2, $3, 0)`,
				pet, photo, env.ShelterA)

			return err
		}); err != nil {
		t.Fatalf("attaching a photo to the pet: %v", err)
	}
	publish(t, env, env.ShelterA, pet)

	if !publiclyVisibleMedia(t, env, photo) {
		t.Fatal("the photo is not public before the soft delete, so the assertion after it " +
			"would pass for the wrong reason")
	}

	if err := db.WithTenant(ctx, env.TenantPool, env.ShelterA,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE media SET deleted_at = now() WHERE id = $1`, photo)

			return err
		}); err != nil {
		t.Fatalf("soft-deleting the photo: %v", err)
	}

	if publiclyVisibleMedia(t, env, photo) {
		t.Error("a soft-deleted media row is still public. Its pet is published, so nothing " +
			"upstream in the chain will ever hide it: media's own policy has to read " +
			"deleted_at itself")
	}
}

// publiclyVisibleMedia reads one media row as app_public: a separate role, a
// separate pool, a read-only transaction and no tenant scope at all.
func publiclyVisibleMedia(t *testing.T, env *dbtest.Env, media uuid.UUID) bool {
	t.Helper()

	var found int
	err := db.WithPublic(context.Background(), env.PublicPool,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				`SELECT count(*) FROM media WHERE id = $1`, media).Scan(&found)
		})
	if err != nil {
		t.Fatalf("reading media as app_public: %v", err)
	}

	return found == 1
}
