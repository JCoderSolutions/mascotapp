package rlstest_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gentleman/mascotapp/apps/api/internal/db"
	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
	"github.com/gentleman/mascotapp/apps/api/internal/db/rlstest"
)

// The A/B isolation suite: ADR-0002's completion rule, one case per tenant
// table. A table without a case here is not finished, and the catalog meta-test
// in catalog_test.go is what turns forgetting one into a failure.
//
// Every case runs as app_tenant, never as the owner. The owner is precisely the
// role that would make a broken policy look like a working one.
func TestTenantIsolation(t *testing.T) {
	env := dbtest.Postgres(t)

	dbtest.RunAB(t, env, abCases()...)
}

// ADR-0002's completion rule, asserted instead of counted by hand: *"a table
// without a passing case is not done"*.
//
// Written at T-01-035, when the phase close-out compared the case list against
// the declared tenant set BY EYE and found it correct. That is exactly the check
// this phase learned not to trust — a hand-comparison is right until the
// migration nobody re-compares. §10 calls the A/B coverage BLOCKING for this
// phase, so it gets a test rather than a reading.
//
// It fails in BOTH directions. A tenant table with no case is a table whose
// isolation nothing proves; a case for a table outside the declared set is a
// case built on a name the declaration does not know, which is how a rename
// leaves a suite green over nothing.
func TestTenantIsolation_CoversEveryDeclaredTenantTable(t *testing.T) {
	t.Parallel()

	covered := map[string]bool{}
	for _, c := range abCases() {
		if covered[c.Name] {
			t.Errorf("two A/B cases are named %q, so one of them is shadowed in the "+
				"subtest output and nobody would see it stop running", c.Name)
		}
		covered[c.Name] = true
	}

	declared := map[string]bool{}
	for _, name := range rlstest.Schema.Tenant {
		declared[name] = true
		if !covered[name] {
			t.Errorf("the tenant table %q has no A/B isolation case. ADR-0002's completion "+
				"rule is that a table without a passing case is not done, and §10 makes this "+
				"BLOCKING for the phase: without a case, nothing proves tenant B cannot read "+
				"or write that table's rows", name)
		}
	}

	for name := range covered {
		if !declared[name] {
			t.Errorf("there is an A/B case for %q, which is not in the declared tenant set. "+
				"Either the declaration is missing a table or the case is built on a name "+
				"that no longer exists — and a case over a table that is not there passes "+
				"while proving nothing", name)
		}
	}
}

// abCases builds the suite once, so the runner and the coverage assertion above
// read the SAME list. Two lists would be one more thing to keep in step.
//
// Declaration order is execution order, and it is load-bearing in three places —
// each noted at its case.
func abCases() []dbtest.TenantTable {
	// shelters goes first on purpose: its case is the only writer of shelter A's
	// row, and memberships' foreign key needs that row. Every fixture is
	// idempotent so any case still passes when run alone with -run.
	parents := newPetChildParents()

	forms := newFormParents()

	applications := newApplicationParents(parents)

	return []dbtest.TenantTable{
		sheltersCase(), membershipsCase(uuid.New()), mediaCase(), petsCase(),
		petMediaCase(parents), petHealthRecordsCase(parents), petStatusHistoryCase(parents),
		formTemplatesCase(), formTemplateVersionsCase(forms), formSubmissionsCase(forms),
		// After petsCase, and that is ordering, not taste. Applications reference
		// pets with ON DELETE RESTRICT, and the pets case probes an unqualified
		// `DELETE FROM pets` as tenant B -- the only shape that catches a
		// permissive DELETE policy. An application seeded on B's pet before that
		// probe turns its refusal into a 23503 about referential integrity,
		// which is not what the case is asserting.
		adoptionApplicationsCase(parents, uuid.New()),
		// Both children of the application, and they must come after its own
		// case for the same reason applications come after pets: they reference
		// it with ON DELETE RESTRICT, and the applications case probes an
		// unqualified DELETE as tenant B.
		applicationEventsCase(applications), applicationNotesCase(applications),
		documentsCase(applications), auditLogCase(),
	}
}

// audit_log is APPEND-ONLY and is the one table in this schema whose identifier
// is a sequence rather than a uuid, so its case inserts WITHOUT one and reads
// back what the sequence issued. That also makes the case depend on the sequence
// grant: without it, this fails to seed rather than failing to isolate.
func auditLogCase() dbtest.TenantTable {
	return dbtest.TenantTable{
		Name:       "audit_log",
		AppendOnly: true,
		Fixture: func(ctx context.Context, env *dbtest.Env) error {
			return seedShelters(ctx, env.OwnerPool, env.ShelterA, env.ShelterB)
		},
		Insert: func(ctx context.Context, tx pgx.Tx, shelterID uuid.UUID) (dbtest.RowKey, error) {
			var id int64
			err := tx.QueryRow(ctx,
				`INSERT INTO audit_log (shelter_id, action, entity_type, entity_id)
				 VALUES ($1, 'pet.published', 'pets', $2)
				 RETURNING id`,
				shelterID, uuid.New()).Scan(&id)
			if err != nil {
				return nil, fmt.Errorf("appending an audit row: %w", err)
			}

			return dbtest.RowKey{"id": id}, nil
		},
	}
}

// documents hangs off BOTH `media` and `adoption_applications`, so its fixture
// reuses the application parents — which already seed the pet and media pair.
func documentsCase(parents *applicationParents) dbtest.TenantTable {
	return dbtest.TenantTable{
		Name:    "documents",
		Fixture: parents.seed,
		Insert: func(ctx context.Context, tx pgx.Tx, shelterID uuid.UUID) (dbtest.RowKey, error) {
			id := uuid.New()
			_, media := parents.pet.idsFor(shelterID)
			_, err := tx.Exec(ctx,
				`INSERT INTO documents (id, shelter_id, media_id, application_id, type)
				 VALUES ($1, $2, $3, $4, 'adoption_contract')`,
				id, shelterID, media, parents.idFor(shelterID))
			if err != nil {
				return nil, fmt.Errorf("filing a document: %w", err)
			}

			return dbtest.RowKey{"id": id}, nil
		},
	}
}

// applicationParents holds one adoption application per shelter for the two
// child cases, created by the OWNER — same reasoning as petChildParents. If
// `Insert` also created the application, tenant B's forged row would die on
// `adoption_applications`' WITH CHECK before it ever reached the child, and the
// case would pass while proving nothing about the child at all.
type applicationParents struct {
	pet       *petChildParents
	applicant uuid.UUID
	byShelter map[uuid.UUID]uuid.UUID
}

func newApplicationParents(pet *petChildParents) *applicationParents {
	return &applicationParents{
		pet:       pet,
		applicant: uuid.New(),
		byShelter: map[uuid.UUID]uuid.UUID{},
	}
}

func (p *applicationParents) idFor(shelter uuid.UUID) uuid.UUID {
	if _, ok := p.byShelter[shelter]; !ok {
		p.byShelter[shelter] = uuid.New()
	}

	return p.byShelter[shelter]
}

func (p *applicationParents) seed(ctx context.Context, env *dbtest.Env) error {
	if err := p.pet.seed(ctx, env); err != nil {
		return err
	}
	if err := seedMemberUser(ctx, env.OwnerPool, p.applicant); err != nil {
		return err
	}

	for _, shelter := range []uuid.UUID{env.ShelterA, env.ShelterB} {
		pet, _ := p.pet.idsFor(shelter)
		if _, err := env.OwnerPool.Exec(ctx,
			`INSERT INTO adoption_applications
			     (id, shelter_id, pet_id, applicant_user_id)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (id) DO NOTHING`,
			p.idFor(shelter), shelter, pet, p.applicant); err != nil {
			return fmt.Errorf("seeding parent application for %s: %w", shelter, err)
		}
	}

	return nil
}

// application_events is APPEND-ONLY, so its case asserts that tenant B's UPDATE
// and DELETE are REFUSED outright rather than merely reaching no rows — the
// distinction that separates a protected table from one whose policy simply
// filtered the row away.
func applicationEventsCase(parents *applicationParents) dbtest.TenantTable {
	return dbtest.TenantTable{
		Name:       "application_events",
		AppendOnly: true,
		Fixture:    parents.seed,
		Insert: func(ctx context.Context, tx pgx.Tx, shelterID uuid.UUID) (dbtest.RowKey, error) {
			id := uuid.New()
			_, err := tx.Exec(ctx,
				`INSERT INTO application_events (id, shelter_id, application_id, type)
				 VALUES ($1, $2, $3, 'status_changed')`,
				id, shelterID, parents.idFor(shelterID))
			if err != nil {
				return nil, fmt.Errorf("appending an application event: %w", err)
			}

			return dbtest.RowKey{"id": id}, nil
		},
	}
}

// application_notes is the ordinary shape: a shelter edits and deletes its own
// notes. The contrast with the case above is what keeps "append-only" meaning
// something rather than describing every table this migration created.
func applicationNotesCase(parents *applicationParents) dbtest.TenantTable {
	return dbtest.TenantTable{
		Name:    "application_notes",
		Fixture: parents.seed,
		Insert: func(ctx context.Context, tx pgx.Tx, shelterID uuid.UUID) (dbtest.RowKey, error) {
			id := uuid.New()
			_, err := tx.Exec(ctx,
				`INSERT INTO application_notes (id, shelter_id, application_id, body)
				 VALUES ($1, $2, $3, 'Called the applicant')`,
				id, shelterID, parents.idFor(shelterID))
			if err != nil {
				return nil, fmt.Errorf("writing an application note: %w", err)
			}

			return dbtest.RowKey{"id": id}, nil
		},
	}
}

// An application's parent pet comes from the same fixture the three pet children
// use, for the reason written on petChildParents: a case that created its own
// parent inside `Insert` would see tenant B's forged attempt die on `pets`' WITH
// CHECK instead of on this table's, and pass while proving nothing.
//
// The applicant is seeded separately because it is a `users` row, and `users` is
// the one table here that belongs to no shelter (D6).
func adoptionApplicationsCase(
	parents *petChildParents, applicant uuid.UUID,
) dbtest.TenantTable {
	return dbtest.TenantTable{
		Name: "adoption_applications",
		Fixture: func(ctx context.Context, env *dbtest.Env) error {
			if err := parents.seed(ctx, env); err != nil {
				return err
			}

			return seedMemberUser(ctx, env.OwnerPool, applicant)
		},
		Insert: func(ctx context.Context, tx pgx.Tx, shelterID uuid.UUID) (dbtest.RowKey, error) {
			id := uuid.New()
			pet, _ := parents.idsFor(shelterID)

			// The pet is the STAMPED shelter's pet, so the composite key is
			// satisfied on the forged attempt too and the policy is what has to
			// refuse it. Using tenant B's own pet there would raise a 23503 that
			// stands in for the 42501 the runner is looking for.
			_, err := tx.Exec(ctx,
				`INSERT INTO adoption_applications
				     (id, shelter_id, pet_id, applicant_user_id)
				 VALUES ($1, $2, $3, $4)`,
				id, shelterID, pet, applicant)
			if err != nil {
				return nil, fmt.Errorf("recording an application: %w", err)
			}

			return dbtest.RowKey{"id": id}, nil
		},
	}
}

// petChildParents holds one pet and one media per shelter for the three child
// cases, created by the OWNER before each case runs.
//
// The parents cannot be created by `Insert`, and that is not a convenience.
// `Insert` is called twice: once as tenant A to seed the row, and once as tenant
// B stamped with A's shelter_id, where it must be refused by THIS table's
// policy. If it also created the parent pet, tenant B's attempt would die on
// `pets`' WITH CHECK before it ever reached the child — the same 42501 the check
// asserts, raised by the wrong table. The case would pass while proving nothing
// about the child at all.
type petChildParents struct{ pet, media map[uuid.UUID]uuid.UUID }

func newPetChildParents() *petChildParents {
	return &petChildParents{
		pet:   map[uuid.UUID]uuid.UUID{},
		media: map[uuid.UUID]uuid.UUID{},
	}
}

// idsFor returns this shelter's parent pet and media, minting them on first ask
// so both tenants get a stable pair for the life of the run.
func (p *petChildParents) idsFor(shelter uuid.UUID) (pet, media uuid.UUID) {
	if _, ok := p.pet[shelter]; !ok {
		p.pet[shelter] = uuid.New()
		p.media[shelter] = uuid.New()
	}

	return p.pet[shelter], p.media[shelter]
}

// seed creates both shelters and a pet and a media row for each, as the owner.
// Idempotent: the container is shared and any case may be re-run alone.
func (p *petChildParents) seed(ctx context.Context, env *dbtest.Env) error {
	if err := seedShelters(ctx, env.OwnerPool, env.ShelterA, env.ShelterB); err != nil {
		return err
	}

	for _, shelter := range []uuid.UUID{env.ShelterA, env.ShelterB} {
		pet, media := p.idsFor(shelter)

		if _, err := env.OwnerPool.Exec(ctx,
			`INSERT INTO pets (id, shelter_id, public_code, name, species_id, size)
			 VALUES ($1, $2, $3, 'Firulais',
			         (SELECT id FROM species WHERE code = 'dog'), 'm')
			 ON CONFLICT (id) DO NOTHING`,
			pet, shelter, "P-"+pet.String()); err != nil {
			return fmt.Errorf("seeding parent pet %s: %w", pet, err)
		}

		if _, err := env.OwnerPool.Exec(ctx,
			`INSERT INTO media (id, shelter_id, kind, storage_key, mime, bytes)
			 VALUES ($1, $2, 'image', $3, 'image/avif', 1024)
			 ON CONFLICT (id) DO NOTHING`,
			media, shelter, "shelters/"+shelter.String()+"/"+media.String()); err != nil {
			return fmt.Errorf("seeding parent media %s: %w", media, err)
		}
	}

	return nil
}

// formParents holds one form template per shelter, created by the OWNER, for the
// `form_template_versions` case — same reasoning as petChildParents: if `Insert`
// also created the template, tenant B's forged row would die on
// `form_templates`' WITH CHECK before it ever reached the child, and the case
// would pass while proving nothing about the child at all.
type formParents struct{ template, version map[uuid.UUID]uuid.UUID }

func newFormParents() *formParents {
	return &formParents{
		template: map[uuid.UUID]uuid.UUID{},
		version:  map[uuid.UUID]uuid.UUID{},
	}
}

func (p *formParents) idFor(shelter uuid.UUID) uuid.UUID {
	if _, ok := p.template[shelter]; !ok {
		p.template[shelter] = uuid.New()
	}

	return p.template[shelter]
}

// versionFor is the PUBLISHED version the submissions case hangs off.
func (p *formParents) versionFor(shelter uuid.UUID) uuid.UUID {
	if _, ok := p.version[shelter]; !ok {
		p.version[shelter] = uuid.New()
	}

	return p.version[shelter]
}

func (p *formParents) seed(ctx context.Context, env *dbtest.Env) error {
	if err := seedShelters(ctx, env.OwnerPool, env.ShelterA, env.ShelterB); err != nil {
		return err
	}

	for _, shelter := range []uuid.UUID{env.ShelterA, env.ShelterB} {
		template := p.idFor(shelter)
		if _, err := env.OwnerPool.Exec(ctx,
			`INSERT INTO form_templates (id, shelter_id, key, name, purpose)
			 VALUES ($1, $2, $3, 'Adoption form', 'adoption_application')
			 ON CONFLICT (id) DO NOTHING`,
			template, shelter, "adoption-"+template.String()); err != nil {
			return fmt.Errorf("seeding parent template %s: %w", template, err)
		}
	}

	return nil
}

func formTemplatesCase() dbtest.TenantTable {
	return dbtest.TenantTable{
		Name: "form_templates",
		Insert: func(ctx context.Context, tx pgx.Tx, shelterID uuid.UUID) (dbtest.RowKey, error) {
			id := uuid.New()
			// A fresh key per call. Reusing one would let a 23505 on
			// UNIQUE (shelter_id, key) stand in for the policy's refusal on the
			// forged insert, and the case would pass with the policy deleted.
			_, err := tx.Exec(ctx,
				`INSERT INTO form_templates (id, shelter_id, key, name, purpose)
				 VALUES ($1, $2, $3, 'Adoption form', 'adoption_application')`,
				id, shelterID, "adoption-"+id.String())
			if err != nil {
				return nil, fmt.Errorf("inserting a form template: %w", err)
			}

			return dbtest.RowKey{"id": id}, nil
		},
	}
}

// The seeded row is a DRAFT, and that is load-bearing rather than incidental.
//
// A published version is frozen by a trigger, so the runner's UPDATE and DELETE
// probes against tenant A's row would come back P0001 from the trigger instead
// of reaching zero rows through the policy. The case would stay green with
// `tenant_isolation` deleted, because something still said no — the same
// wrong-layer trap petChildParents exists to avoid. Immutability has its own
// test; this one is about the policy.
func formTemplateVersionsCase(parents *formParents) dbtest.TenantTable {
	return dbtest.TenantTable{
		Name:    "form_template_versions",
		Fixture: parents.seed,
		Insert: func(ctx context.Context, tx pgx.Tx, shelterID uuid.UUID) (dbtest.RowKey, error) {
			id := uuid.New()
			template := parents.idFor(shelterID)
			_, err := tx.Exec(ctx,
				`INSERT INTO form_template_versions
				     (id, shelter_id, template_id, version, definition)
				 VALUES ($1, $2, $3, $4, '{"schemaVersion":1}'::jsonb)`,
				id, shelterID, template, nextFormVersion())
			if err != nil {
				return nil, fmt.Errorf("inserting a form template version: %w", err)
			}

			return dbtest.RowKey{"id": id}, nil
		},
	}
}

// The parent version is PUBLISHED here, unlike the draft the versions case
// seeds. Nothing freezes a submission, so there is no trigger to stand in for
// the policy — and a submission against an unpublished form is not a thing the
// product has.
func formSubmissionsCase(parents *formParents) dbtest.TenantTable {
	return dbtest.TenantTable{
		Name: "form_submissions",
		Fixture: func(ctx context.Context, env *dbtest.Env) error {
			if err := parents.seed(ctx, env); err != nil {
				return err
			}

			for _, shelter := range []uuid.UUID{env.ShelterA, env.ShelterB} {
				if _, err := env.OwnerPool.Exec(ctx,
					// The version NUMBER comes from the same counter the versions
					// case uses. A literal 1 collides with whatever that case
					// already wrote for this template, and the 23505 surfaces as
					// a fixture failure with nothing to do with isolation.
					`INSERT INTO form_template_versions
					     (id, shelter_id, template_id, version, definition, published_at)
					 VALUES ($1, $2, $3, $4, '{"schemaVersion":1}'::jsonb, now())
					 ON CONFLICT (id) DO NOTHING`,
					parents.versionFor(shelter), shelter, parents.idFor(shelter),
					nextFormVersion()); err != nil {
					return fmt.Errorf("seeding parent version for %s: %w", shelter, err)
				}
			}

			return nil
		},
		Insert: func(ctx context.Context, tx pgx.Tx, shelterID uuid.UUID) (dbtest.RowKey, error) {
			id := uuid.New()
			_, err := tx.Exec(ctx,
				`INSERT INTO form_submissions
				     (id, shelter_id, template_version_id, answers)
				 VALUES ($1, $2, $3, '{"full_name":"Ana"}'::jsonb)`,
				id, shelterID, parents.versionFor(shelterID))
			if err != nil {
				return nil, fmt.Errorf("recording a submission: %w", err)
			}

			return dbtest.RowKey{"id": id}, nil
		},
	}
}

// nextFormVersion hands out a distinct version number per call, so the forged
// insert cannot collide on UNIQUE (shelter_id, template_id, version) — a 23505
// there would stand in for the policy's refusal exactly like a reused key would.
func nextFormVersion() int {
	formVersionCounter++

	return formVersionCounter
}

var formVersionCounter int

// pet_media is the join row, and the one table in this schema whose identity is
// a PAIR rather than an id — §4.3 gives it no `id`, and a pure join row needs no
// separate one. RowKey was built as a column map for exactly this.
func petMediaCase(parents *petChildParents) dbtest.TenantTable {
	return dbtest.TenantTable{
		Name:    "pet_media",
		Fixture: parents.seed,
		Insert: func(ctx context.Context, tx pgx.Tx, shelterID uuid.UUID) (dbtest.RowKey, error) {
			pet, media := parents.idsFor(shelterID)
			_, err := tx.Exec(ctx,
				`INSERT INTO pet_media (pet_id, media_id, shelter_id, position)
				 VALUES ($1, $2, $3, 0)`,
				pet, media, shelterID)
			if err != nil {
				return nil, fmt.Errorf("attaching media to a pet: %w", err)
			}

			return dbtest.RowKey{"pet_id": pet, "media_id": media}, nil
		},
	}
}

func petHealthRecordsCase(parents *petChildParents) dbtest.TenantTable {
	return dbtest.TenantTable{
		Name:    "pet_health_records",
		Fixture: parents.seed,
		Insert: func(ctx context.Context, tx pgx.Tx, shelterID uuid.UUID) (dbtest.RowKey, error) {
			id := uuid.New()
			pet, _ := parents.idsFor(shelterID)
			_, err := tx.Exec(ctx,
				`INSERT INTO pet_health_records
				     (id, shelter_id, pet_id, type, occurred_on, description)
				 VALUES ($1, $2, $3, 'vaccine', current_date, 'Rabies')`,
				id, shelterID, pet)
			if err != nil {
				return nil, fmt.Errorf("inserting a health record: %w", err)
			}

			return dbtest.RowKey{"id": id}, nil
		},
	}
}

// pet_status_history is APPEND-ONLY (LT-5), so its case asserts that tenant B's
// UPDATE and DELETE are refused outright rather than merely reaching no rows.
func petStatusHistoryCase(parents *petChildParents) dbtest.TenantTable {
	return dbtest.TenantTable{
		Name:       "pet_status_history",
		AppendOnly: true,
		Fixture:    parents.seed,
		Insert: func(ctx context.Context, tx pgx.Tx, shelterID uuid.UUID) (dbtest.RowKey, error) {
			id := uuid.New()
			pet, _ := parents.idsFor(shelterID)
			_, err := tx.Exec(ctx,
				`INSERT INTO pet_status_history
				     (id, shelter_id, pet_id, from_status, to_status, reason)
				 VALUES ($1, $2, $3, 'draft', 'available', 'ready for adoption')`,
				id, shelterID, pet)
			if err != nil {
				return nil, fmt.Errorf("appending a status change: %w", err)
			}

			return dbtest.RowKey{"id": id}, nil
		},
	}
}

// media is the plain shape: its own `shelter_id`, the standard direct policy,
// nothing derived and nothing denormalised from a parent. It is the first table
// in this suite that the policy template covers with no substitution at all.
func mediaCase() dbtest.TenantTable {
	return dbtest.TenantTable{
		Name: "media",
		Fixture: func(ctx context.Context, env *dbtest.Env) error {
			// media.shelter_id is a foreign key, so both tenants need a row.
			return seedShelters(ctx, env.OwnerPool, env.ShelterA, env.ShelterB)
		},
		Insert: func(ctx context.Context, tx pgx.Tx, shelterID uuid.UUID) (dbtest.RowKey, error) {
			id := uuid.New()
			_, err := tx.Exec(ctx,
				`INSERT INTO media (id, shelter_id, kind, storage_key, mime, bytes)
				 VALUES ($1, $2, 'image', $3, 'image/avif', 1024)`,
				id, shelterID, "shelters/"+shelterID.String()+"/"+id.String())
			if err != nil {
				return nil, fmt.Errorf("inserting a media row: %w", err)
			}

			return dbtest.RowKey{"id": id}, nil
		},
	}
}

// pets is the catalog core. Its A/B case covers the tenant policy only; the
// public_catalog policy that sits beside it is a different role and a different
// assertion, and lives in pets_test.go.
func petsCase() dbtest.TenantTable {
	return dbtest.TenantTable{
		Name: "pets",
		Fixture: func(ctx context.Context, env *dbtest.Env) error {
			return seedShelters(ctx, env.OwnerPool, env.ShelterA, env.ShelterB)
		},
		Insert: func(ctx context.Context, tx pgx.Tx, shelterID uuid.UUID) (dbtest.RowKey, error) {
			id := uuid.New()
			_, err := tx.Exec(ctx,
				`INSERT INTO pets (id, shelter_id, public_code, name, species_id, size)
				 VALUES ($1, $2, $3, 'Firulais',
				         (SELECT id FROM species WHERE code = 'dog'), 'm')`,
				id, shelterID, "P-"+uuid.NewString())
			if err != nil {
				return nil, fmt.Errorf("inserting a pet: %w", err)
			}

			return dbtest.RowKey{"id": id}, nil
		},
	}
}

// shelters is scoped by its own id rather than by a shelter_id column, which is
// the one substitution the policy template allows: a shelter's rows ARE the
// tenants. A consequence worth stating -- tenant A can only ever insert exactly
// one row here, the one whose id is A.
func sheltersCase() dbtest.TenantTable {
	return dbtest.TenantTable{
		Name:         "shelters",
		TenantColumn: "id",
		// Because tenant A's row IS shelter A, this case can only work if it is
		// the one that creates it — and the container is shared, so any earlier
		// test that seeds env.ShelterA takes that away. What comes back then is
		// a bare `duplicate key value violates "shelters_pkey"` from an insert
		// that looks perfectly correct, which is a long way from the cause.
		//
		// The guard costs one query and names it. T-01-021 hit exactly this:
		// child_orphan_test.go sorts first in the package, so it seeded shelter
		// A before this case ever ran.
		Fixture: func(ctx context.Context, env *dbtest.Env) error {
			var taken bool
			if err := env.OwnerPool.QueryRow(ctx,
				`SELECT EXISTS (SELECT 1 FROM shelters WHERE id = $1)`,
				env.ShelterA).Scan(&taken); err != nil {
				return fmt.Errorf("checking whether shelter A already exists: %w", err)
			}
			if taken {
				return fmt.Errorf(
					"shelter A (%s) already exists before the shelters case ran, so its "+
						"insert can only fail on the primary key. Some other test in this "+
						"package seeded it first — this case scopes by `id`, so it MUST be "+
						"the one that creates tenant A. Give that test its own shelter "+
						"instead of borrowing env.ShelterA", env.ShelterA)
			}

			return nil
		},
		Insert: func(ctx context.Context, tx pgx.Tx, shelterID uuid.UUID) (dbtest.RowKey, error) {
			// A fresh slug per call, so the only unique constraint the forged
			// insert can collide with is the primary key. Reusing a slug would
			// let a 23505 on `slug` stand in for the policy's refusal, and the
			// case would pass with the policy deleted.
			_, err := tx.Exec(ctx,
				`INSERT INTO shelters (id, slug, display_name) VALUES ($1, $2, $3)`,
				shelterID, "shelter-"+uuid.NewString(), "A Shelter")
			if err != nil {
				return nil, fmt.Errorf("inserting a shelter: %w", err)
			}

			return dbtest.RowKey{"id": shelterID}, nil
		},
	}
}

// memberships denormalises shelter_id and takes the standard direct policy.
//
// memberUser is passed in rather than generated inside so the same user backs
// both the seeded row and the forged one; see
// TestForgedInsert_IsRefusedBeforeAnyUniqueViolation for why that collision is
// wanted rather than avoided.
func membershipsCase(memberUser uuid.UUID) dbtest.TenantTable {
	return dbtest.TenantTable{
		Name: "memberships",
		Fixture: func(ctx context.Context, env *dbtest.Env) error {
			// Both shelters, not just A: the case has to pass when it is run
			// alone with -run, and then the shelters case above never ran.
			if err := seedShelters(ctx, env.OwnerPool, env.ShelterA, env.ShelterB); err != nil {
				return err
			}

			return seedMemberUser(ctx, env.OwnerPool, memberUser)
		},
		Insert: func(ctx context.Context, tx pgx.Tx, shelterID uuid.UUID) (dbtest.RowKey, error) {
			id := uuid.New()
			_, err := tx.Exec(ctx,
				`INSERT INTO memberships (id, user_id, shelter_id, role, status)
				 VALUES ($1, $2, $3, 'volunteer', 'active')`,
				id, memberUser, shelterID)
			if err != nil {
				return nil, fmt.Errorf("inserting a membership: %w", err)
			}

			return dbtest.RowKey{"id": id}, nil
		},
	}
}

// seedMemberUser creates the user a membership points at.
//
// It runs as the OWNER, which in this container is a superuser and therefore
// bypasses row security. That is the only way to create a `users` row at all in
// this phase: app_tenant deliberately holds SELECT and nothing else on it, and
// FORCE ROW LEVEL SECURITY would otherwise deny even the owner, since every
// policy on the table is scoped TO app_tenant. Phase 02's registration flow owns
// the real write path.
//
// Idempotent on purpose: the container is shared across the package and a case
// may be re-run alone.
func seedMemberUser(ctx context.Context, owner *pgxpool.Pool, memberUser uuid.UUID) error {
	return seedUser(ctx, owner, memberUser, fmt.Sprintf("member-%s@example.org", memberUser))
}

// seedUser creates one user with a caller-chosen address, which the email tests
// need and seedMemberUser does not.
func seedUser(ctx context.Context, owner *pgxpool.Pool, id uuid.UUID, email string) error {
	_, err := owner.Exec(ctx,
		`INSERT INTO users (id, email, full_name) VALUES ($1, $2, $3)
		 ON CONFLICT (id) DO NOTHING`,
		id, email, "A Member")
	if err != nil {
		return fmt.Errorf("seeding user %s: %w", id, err)
	}

	return nil
}

// seedMembership joins a user to a shelter with an 'active' membership. As the
// owner, again: app_tenant could only ever create a membership in the shelter
// it is already scoped to, and these tests need to arrange memberships in
// shelters they are NOT scoped to in order to prove those rows stay invisible.
func seedMembership(ctx context.Context, owner *pgxpool.Pool, user, shelter uuid.UUID) error {
	return seedMembershipWithStatus(ctx, owner, user, shelter, "active")
}

// seedMembershipWithStatus is seedMembership's sibling for the tests that need
// a status other than 'active' -- the member_visible_users policy's own
// visibility grid, in particular, which is exactly what proves 'invited' and
// 'revoked' rows stay invisible while existing callers of seedMembership keep
// getting the 'active' row they always did.
func seedMembershipWithStatus(
	ctx context.Context, owner *pgxpool.Pool, user, shelter uuid.UUID, status string,
) error {
	_, err := owner.Exec(ctx,
		`INSERT INTO memberships (id, user_id, shelter_id, role, status)
		 VALUES ($1, $2, $3, 'volunteer', $4)
		 ON CONFLICT (user_id, shelter_id) DO NOTHING`,
		uuid.New(), user, shelter, status)
	if err != nil {
		return fmt.Errorf("seeding a %s membership for %s in %s: %w", status, user, shelter, err)
	}

	return nil
}

// seedShelters creates the tenant rows every foreign key in this schema
// eventually points at. Same owner-as-superuser reasoning as above, and the
// conflict clause is what lets it coexist with the shelters A/B case, which
// creates shelter A itself.
func seedShelters(ctx context.Context, owner *pgxpool.Pool, ids ...uuid.UUID) error {
	for _, id := range ids {
		_, err := owner.Exec(ctx,
			`INSERT INTO shelters (id, slug, display_name) VALUES ($1, $2, $3)
			 ON CONFLICT (id) DO NOTHING`,
			id, "seed-"+uuid.NewString(), "Seeded Shelter")
		if err != nil {
			return fmt.Errorf("seeding shelter %s: %w", id, err)
		}
	}

	return nil
}

// A forged insert has to be refused by the POLICY, not by a unique index that
// happens to fire on the same row.
//
// This is not pedantry about which error comes back. If PostgreSQL checked
// uniqueness first, tenant B would learn that a row with tenant A's key exists
// -- an existence oracle over another tenant's data, delivered by the very
// statement the policy refused. And in the A/B runner it would be worse than a
// leak: a 23505 would keep the case green with the policy deleted, because
// something still said no.
//
// The suite depends on this ordering, so it is asserted rather than assumed.
func TestForgedInsert_IsRefusedBeforeAnyUniqueViolation(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	memberUser := uuid.New()
	if err := seedShelters(ctx, env.OwnerPool, env.ShelterA, env.ShelterB); err != nil {
		t.Fatalf("seeding shelters: %v", err)
	}
	if err := seedMemberUser(ctx, env.OwnerPool, memberUser); err != nil {
		t.Fatalf("seeding the member user: %v", err)
	}

	// Tenant A's membership, which the forged insert below duplicates on
	// UNIQUE (user_id, shelter_id).
	err := db.WithTenant(ctx, env.TenantPool, env.ShelterA,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO memberships (id, user_id, shelter_id, role, status)
				 VALUES ($1, $2, $3, 'volunteer', 'active')`,
				uuid.New(), memberUser, env.ShelterA)

			return err
		})
	if err != nil {
		t.Fatalf("seeding tenant A's membership: %v", err)
	}

	const (
		insufficientPrivilege = "42501" // the WITH CHECK refusal
		uniqueViolation       = "23505"
	)

	// The unique key has to actually bite, or the assertion below is vacuous:
	// with UNIQUE (user_id, shelter_id) dropped, the forged insert would still
	// come back 42501 and this test would keep passing while proving nothing
	// about ordering. So prove the collision is real first, from tenant A's own
	// scope where the policy cannot be what refuses it.
	err = db.WithTenant(ctx, env.TenantPool, env.ShelterA,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO memberships (id, user_id, shelter_id, role, status)
				 VALUES ($1, $2, $3, 'volunteer', 'active')`,
				uuid.New(), memberUser, env.ShelterA)

			return err
		})

	var dupErr *pgconn.PgError
	if !errors.As(err, &dupErr) || dupErr.Code != uniqueViolation {
		t.Fatalf("a second membership for the same (user, shelter) was not refused by "+
			"UNIQUE (user_id, shelter_id) (%s), so the ordering assertion below would be "+
			"vacuous: %v", uniqueViolation, err)
	}

	err = db.WithTenant(ctx, env.TenantPool, env.ShelterB,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO memberships (id, user_id, shelter_id, role, status)
				 VALUES ($1, $2, $3, 'volunteer', 'active')`,
				uuid.New(), memberUser, env.ShelterA)

			return err
		})
	if err == nil {
		t.Fatal("tenant B inserted a membership stamped with tenant A's shelter_id")
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("the refusal did not come from PostgreSQL: %v", err)
	}

	if pgErr.Code == uniqueViolation {
		t.Fatalf("the unique index refused the forged row before the policy did (%s). "+
			"Tenant B just learned that a row with tenant A's key exists, and every A/B "+
			"case whose forged insert collides on a unique key would stay green with the "+
			"policy deleted", uniqueViolation)
	}
	if pgErr.Code != insufficientPrivilege {
		t.Fatalf("the forged insert failed with %s, want the WITH CHECK refusal %s: %v",
			pgErr.Code, insufficientPrivilege, err)
	}
}

// The spec requires every primary key in this capability to be `uuid NOT NULL`
// with no database default: identifiers are time-ordered UUID v7 generated in
// Go, and a database default would make that quietly optional.
//
// The table list is this capability's, from data-model-core, rather than the
// whole catalog -- audit_log is a BIGSERIAL by design and belongs to a
// different requirement. Tables that have not landed yet are counted and
// reported, so a green run over zero tables cannot read as proof.
func TestPrimaryKeys_CarryNoDatabaseDefault(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	capability := []string{"shelters", "users", "memberships", "refresh_tokens", "media"}

	checked, absent := 0, 0
	for _, table := range capability {
		var exists bool
		if err := env.OwnerPool.QueryRow(ctx,
			`SELECT to_regclass('public.' || $1) IS NOT NULL`, table).Scan(&exists); err != nil {
			t.Fatalf("looking for %s: %v", table, err)
		}
		if !exists {
			absent++

			continue
		}
		checked++

		rows, err := env.OwnerPool.Query(ctx, `
			SELECT a.attname,
			       format_type(a.atttypid, a.atttypmod),
			       a.attnotnull,
			       d.adbin IS NOT NULL
			FROM pg_index i
			JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY (i.indkey)
			LEFT JOIN pg_attrdef d ON d.adrelid = i.indrelid AND d.adnum = a.attnum
			WHERE i.indrelid = ('public.' || $1)::regclass
			  AND i.indisprimary`, table)
		if err != nil {
			t.Fatalf("reading %s's primary key: %v", table, err)
		}

		columns := 0
		for rows.Next() {
			var name, typ string
			var notNull, hasDefault bool
			if err := rows.Scan(&name, &typ, &notNull, &hasDefault); err != nil {
				rows.Close()
				t.Fatalf("scanning %s's primary key: %v", table, err)
			}
			columns++

			if typ != "uuid" {
				t.Errorf("%s.%s is %s, want uuid", table, name, typ)
			}
			if !notNull {
				t.Errorf("%s.%s is nullable", table, name)
			}
			if hasDefault {
				t.Errorf("%s.%s has a database default. Identifiers are UUID v7 generated "+
					"in Go before the insert (D3); a default makes that optional and lets "+
					"a v4 in the first time somebody forgets", table, name)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Fatalf("reading %s's primary key: %v", table, err)
		}
		if columns == 0 {
			t.Errorf("%s has no primary key at all", table)
		}
	}

	if checked == 0 {
		t.Fatalf("none of the %d tables in this capability exist, so this proved nothing",
			len(capability))
	}
	t.Logf("checked %d of %d primary keys; %d table(s) not migrated yet", checked,
		len(capability), absent)
}

// The access contract of this phase's migrations, pinned exactly: which policy
// exists on which table, for which command, for which role — and which
// privileges each role holds.
//
// The catalog meta-test counts policies; it cannot see who a policy is FOR.
// Mutation testing on 2026-08-30 showed what that costs: retargeting
// member_visible_users from app_tenant to app_public broke nothing, and neither
// did widening the users grant from SELECT to full write. Both leave a migration
// that reads as correct and grants the wrong thing.
//
// Judgment Day (T-01-016) raised the same gap independently, as finding B2, and
// pointed out that the fix so far was table-by-table: this test covered 00002's
// four tables and nothing else, so `media` would have crossed it uncovered.
// T-01-017 widened it. **Every migration from here adds its rows to both tables
// below** — that is the price of the meta-test not being able to see roles, and
// it is cheaper than the alternative, which is not noticing.
func TestTenancyPolicies_ApplyToTheRightRoleAndCommand(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	// polcmd: '*' is ALL, 'r' SELECT, 'a' INSERT, 'w' UPDATE, 'd' DELETE.
	type policy struct{ table, name, cmd, roles string }

	want := []policy{
		{"adoption_applications", "tenant_isolation", "*", "app_tenant"},
		// Two per-command policies, and the ABSENCE of an UPDATE or DELETE one
		// is the assertion: under FORCE, a command with no permissive policy
		// matches zero rows for the owner too.
		{"application_events", "tenant_append", "a", "app_tenant"},
		{"application_events", "tenant_read", "r", "app_tenant"},
		{"application_notes", "tenant_isolation", "*", "app_tenant"},
		{"audit_log", "tenant_append", "a", "app_tenant"},
		{"audit_log", "tenant_read", "r", "app_tenant"},
		{"breeds", "reference_readable", "r", "app_public,app_tenant"},
		{"documents", "tenant_isolation", "*", "app_tenant"},
		{"form_submissions", "tenant_isolation", "*", "app_tenant"},
		{"form_template_versions", "tenant_isolation", "*", "app_tenant"},
		{"form_templates", "tenant_isolation", "*", "app_tenant"},
		{"media", "public_catalog", "r", "app_public"},
		{"media", "tenant_isolation", "*", "app_tenant"},
		// Phase 02 (00013, P2-D3): the auth door reads which shelters a user
		// belongs to in order to mint a claim, scoped to its own rows, and
		// never writes here. "auth_" sorts before "tenant_" under COLLATE "C".
		{"memberships", "auth_own_memberships", "r", "app_auth"},
		{"memberships", "tenant_isolation", "*", "app_tenant"},
		{"pet_health_records", "tenant_isolation", "*", "app_tenant"},
		{"pet_media", "public_catalog", "r", "app_public"},
		{"pet_media", "tenant_isolation", "*", "app_tenant"},
		// pet_status_history is append-only, so it is the first table in this
		// schema whose tenant access is TWO per-command policies instead of one
		// FOR ALL. The absence of an UPDATE or DELETE policy is the second
		// layer: even with the grant restored by mistake, under FORCE there is
		// no permissive policy for those commands to match.
		{"pet_status_history", "tenant_append", "a", "app_tenant"},
		{"pet_status_history", "tenant_read", "r", "app_tenant"},
		{"pets", "public_catalog", "r", "app_public"},
		{"pets", "tenant_isolation", "*", "app_tenant"},
		// refresh_tokens gained its policy in Phase 02 (00013, P2-D2): the auth
		// role reads and writes it, scoped by app.user_id, and app_tenant /
		// app_public still hold no grant on it at all — see the privilege
		// inventory below.
		{"refresh_tokens", "auth_own_sessions", "*", "app_auth"},
		{"shelters", "tenant_isolation", "*", "app_tenant"},
		{"species", "reference_readable", "r", "app_public,app_tenant"},
		// TWO permissive policies on users, and they OR together: one names
		// membership as a reason to be visible, the other names having applied.
		// Kept separate on purpose -- a single policy with an OR inside it is one
		// edit away from widening both paths at once (D6).
		{"users", "applicant_visible_users", "r", "app_tenant"},
		// Phase 02 (00013, P2-D3) adds app_auth's three policies on users: a
		// USING(true) read narrowed by column grant (the lookup predicate
		// genuinely cannot be written), a self-scoped UPDATE, and an open
		// INSERT for registration. "auth_" sorts before "member_" under
		// COLLATE "C", so these three land between the two app_tenant rows.
		{"users", "auth_lookup_users", "r", "app_auth"},
		{"users", "auth_own_user", "w", "app_auth"},
		{"users", "auth_register_user", "a", "app_auth"},
		{"users", "member_visible_users", "r", "app_tenant"},
	}

	rows, err := env.OwnerPool.Query(ctx, `
		SELECT c.relname,
		       p.polname,
		       p.polcmd::text,
		       COALESCE((SELECT string_agg(r.rolname, ',' ORDER BY r.rolname)
		                 FROM pg_roles r WHERE r.oid = ANY (p.polroles)), 'PUBLIC')
		FROM pg_policy p
		JOIN pg_class c ON c.oid = p.polrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public'
		  -- Bound to the DECLARATION rather than to a list written out here.
		  -- A hand-written list has to be extended by whoever adds a table, and
		  -- forgetting is silent in the worst direction: the new table's
		  -- policies are simply never read, so the inventory reports a clean
		  -- schema it never looked at. T-01-023 landed two tables and hit
		  -- exactly that.
		  AND c.relname = ANY ($1)
		-- COLLATE "C" so the order is byte order and not the container's locale.
		-- Under en_US.UTF-8 the underscore is ignored at the primary level, which
		-- sorts pet_status_history AFTER pets; under C it sorts before. The
		-- expectation above must not depend on which image the test happens to
		-- pull.
		ORDER BY c.relname COLLATE "C", p.polname COLLATE "C"`,
		rlstest.Schema.ModelTables())
	if err != nil {
		t.Fatalf("reading the policy inventory: %v", err)
	}
	defer rows.Close()

	var got []policy
	for rows.Next() {
		var p policy
		if err := rows.Scan(&p.table, &p.name, &p.cmd, &p.roles); err != nil {
			t.Fatalf("scanning a policy: %v", err)
		}
		got = append(got, p)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the policy inventory: %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("policy inventory = %+v, want %+v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("policy %d is %+v, want %+v", i, got[i], w)
		}
	}

	// The grants are the second layer, and a layer nobody asserts is a layer
	// that quietly stops existing.
	for _, tc := range []struct {
		role, table, privilege string
		want                   bool
		why                    string
	}{
		{"app_tenant", "shelters", "SELECT", true, ""},
		{"app_tenant", "shelters", "INSERT", true, ""},
		{"app_tenant", "memberships", "DELETE", true, ""},
		{"app_tenant", "users", "SELECT", true, ""},
		{"app_tenant", "users", "INSERT", false,
			"no tenant creates a user in this phase; Phase 02's registration flow owns that"},
		{"app_tenant", "users", "UPDATE", false, "same"},
		{"app_tenant", "users", "DELETE", false, "same"},
		{"app_tenant", "refresh_tokens", "SELECT", false,
			"refresh_tokens is reachable only through app_auth (P2-D1); app_tenant holds no " +
				"grant on it at all"},
		{"app_public", "refresh_tokens", "SELECT", false, "same door, same reason"},
		{"app_auth", "refresh_tokens", "SELECT", true, ""},
		{"app_auth", "refresh_tokens", "INSERT", true, ""},
		{"app_auth", "refresh_tokens", "UPDATE", true, ""},
		{"app_auth", "refresh_tokens", "DELETE", true, ""},
		{"app_auth", "users", "SELECT", false,
			"VERIFIED live on PG 17: a column grant is NOT a table privilege, so " +
				"has_table_privilege is false here BY DESIGN. Expecting false is the " +
				"stronger assertion -- it goes red if anyone widens the credential grant " +
				"to the whole table. The columns are asserted precisely in auth_door_test.go"},
		{"app_auth", "users", "INSERT", false,
			"column grant on (id, email, password_hash, full_name, phone); same reason"},
		{"app_auth", "users", "UPDATE", false,
			"column grant on password_hash, totp_secret_enc, email_verified_at, " +
				"last_login_at, updated_at; same reason"},
		{"app_auth", "users", "DELETE", false, "no account-deletion path exists in this phase"},
		{"app_auth", "memberships", "SELECT", false,
			"column grant on (id, user_id, shelter_id, role, status); the row scope is " +
				"auth_own_memberships and is asserted in auth_door_test.go"},
		{"app_auth", "memberships", "INSERT", false,
			"app_auth never writes memberships; the founder's membership insert runs under " +
				"app_tenant inside registration's second transaction (P2-D6)"},
		{"app_auth", "shelters", "SELECT", false,
			"app_auth never gets a policy on shelters; shelter rows are read under tenant " +
				"scope once the claim exists (P2-D3)"},
		{"app_tenant", "media", "SELECT", true, ""},
		{"app_tenant", "media", "INSERT", true, ""},
		{"app_tenant", "media", "UPDATE", true, ""},
		{"app_tenant", "media", "DELETE", true, ""},
		{"app_tenant", "species", "SELECT", true, ""},
		{"app_public", "species", "SELECT", true,
			"the public catalog needs the same breed list the shelters use"},
		{"app_tenant", "species", "INSERT", false,
			"reference data is global: one shelter must not edit it for everyone else"},
		{"app_tenant", "species", "UPDATE", false, "same"},
		{"app_tenant", "species", "DELETE", false, "same"},
		{"app_public", "breeds", "SELECT", true, ""},
		{"app_public", "breeds", "INSERT", false, "app_public is anonymous and read-only"},
		{"app_tenant", "pets", "SELECT", true, ""},
		{"app_tenant", "pets", "DELETE", true, ""},
		{"app_public", "pets", "SELECT", true,
			"the public catalog is the product; the policy is what narrows it"},
		{"app_public", "pets", "UPDATE", false, "app_public is anonymous and read-only"},
		{"app_public", "media", "SELECT", true,
			"deferred through 00003 and 00005, paid in 00006: media is public only THROUGH " +
				"an attachment to a public pet, and the attachment table is pet_media"},
		{"app_public", "media", "UPDATE", false, "app_public is anonymous and read-only"},
		{"app_tenant", "pet_media", "SELECT", true, ""},
		{"app_tenant", "pet_media", "INSERT", true, ""},
		{"app_tenant", "pet_media", "DELETE", true,
			"detaching a photo from a pet is ordinary editing, not history"},
		{"app_public", "pet_media", "SELECT", true,
			"media's public policy reads pet_media, and a policy's subquery is itself " +
				"filtered by the referenced table's RLS. Without this grant and its own " +
				"policy the chain returns nothing and no photo is ever public"},
		{"app_public", "pet_media", "INSERT", false, "app_public is anonymous and read-only"},
		{"app_tenant", "pet_health_records", "SELECT", true, ""},
		{"app_tenant", "pet_health_records", "UPDATE", true,
			"a vet's typo is corrected in place; health records are not history"},
		{"app_public", "pet_health_records", "SELECT", false,
			"a pet's medical file is not part of the public catalog"},
		{"app_tenant", "pet_status_history", "SELECT", true, ""},
		{"app_tenant", "pet_status_history", "INSERT", true, ""},
		{"app_tenant", "pet_status_history", "UPDATE", false,
			"append-only (LT-5): the trail of how an animal moved through the shelter is " +
				"not editable by the shelter that wrote it"},
		{"app_tenant", "pet_status_history", "DELETE", false, "same"},
		{"app_tenant", "pet_status_history", "TRUNCATE", false,
			"TRUNCATE is the one write that no row-level policy can ever see"},
		{"app_public", "pet_status_history", "SELECT", false,
			"the public catalog shows the animal, not its case file"},
		{"app_tenant", "form_templates", "SELECT", true, ""},
		{"app_tenant", "form_templates", "INSERT", true, ""},
		{"app_tenant", "form_templates", "UPDATE", true,
			"renaming a form, or retiring it with is_active, is ordinary editing"},
		{"app_tenant", "form_templates", "DELETE", true, ""},
		{"app_public", "form_templates", "SELECT", false,
			"forms are shelter-side in this phase; the public catalog shows animals"},
		{"app_tenant", "form_template_versions", "SELECT", true, ""},
		{"app_tenant", "form_template_versions", "INSERT", true, ""},
		{"app_tenant", "form_template_versions", "UPDATE", true,
			"a DRAFT version has to be editable, so unlike pet_status_history the grant " +
				"cannot be the layer that freezes anything — the trigger decides per row"},
		{"app_tenant", "form_template_versions", "DELETE", true,
			"same: a draft is discardable, a published version is not"},
		{"app_tenant", "form_template_versions", "TRUNCATE", false,
			"it would take the published rows with it, and no row-level mechanism sees a " +
				"truncate coming"},
		{"app_public", "form_template_versions", "SELECT", false, "same as its template"},
		{"app_tenant", "form_submissions", "SELECT", true, ""},
		{"app_tenant", "form_submissions", "INSERT", true, ""},
		{"app_tenant", "form_submissions", "DELETE", true,
			"§5.4 requires a retention policy and a subject-deletion path; a table nobody " +
				"can delete from cannot honour either"},
		{"app_tenant", "form_submissions", "TRUNCATE", false,
			"no row-level mechanism sees a truncate coming"},
		{"app_public", "form_submissions", "SELECT", false,
			"an adoption application is the most sensitive record in this schema"},
		{"app_tenant", "application_events", "SELECT", true, ""},
		{"app_tenant", "application_events", "INSERT", true, ""},
		{"app_tenant", "application_events", "UPDATE", false,
			"append-only: the timeline of an adoption case is what an audit reads, and " +
				"a record of what happened that can be rewritten is not one"},
		{"app_tenant", "application_events", "DELETE", false, "same"},
		{"app_tenant", "application_events", "TRUNCATE", false,
			"TRUNCATE is the one write that no row-level policy can ever see"},
		{"app_public", "application_events", "SELECT", false,
			"the public catalog shows animals, not the case files behind them"},
		{"app_tenant", "application_notes", "SELECT", true, ""},
		{"app_tenant", "application_notes", "INSERT", true, ""},
		{"app_tenant", "application_notes", "UPDATE", true,
			"a note is working memory, not history: people make typos and change their " +
				"minds. The trail is application_events"},
		{"app_tenant", "application_notes", "DELETE", true, "same"},
		{"app_tenant", "application_notes", "TRUNCATE", false,
			"nothing in this product ever empties a table in one statement"},
		{"app_public", "application_notes", "SELECT", false,
			"an internal note is the shelter writing about a person"},
		{"app_tenant", "audit_log", "SELECT", true, ""},
		{"app_tenant", "audit_log", "INSERT", true, ""},
		{"app_tenant", "audit_log", "UPDATE", false,
			"append-only: an audit trail that can be edited answers no question it was " +
				"written to answer"},
		{"app_tenant", "audit_log", "DELETE", false, "same"},
		{"app_tenant", "audit_log", "TRUNCATE", false,
			"TRUNCATE is the one write that no row-level policy can ever see"},
		{"app_public", "audit_log", "SELECT", false,
			"the audit trail is forensic, and it carries before/after images of rows the " +
				"public was never shown"},
		{"app_tenant", "documents", "SELECT", true, ""},
		{"app_tenant", "documents", "INSERT", true, ""},
		{"app_tenant", "documents", "UPDATE", true,
			"correcting a type or attaching a signature is ordinary work; what must not " +
				"be rewritable is the record of what HAPPENED, and that is application_events"},
		{"app_tenant", "documents", "DELETE", true,
			"a shelter that filed the wrong certificate has to be able to unfile it, and " +
				"§5.4's subject-deletion path reaches documents too"},
		{"app_tenant", "documents", "TRUNCATE", false,
			"no row-level mechanism sees a truncate coming"},
		{"app_public", "documents", "SELECT", false,
			"a signed contract carries the adopter's name and address"},
		{"app_tenant", "adoption_applications", "SELECT", true, ""},
		{"app_tenant", "adoption_applications", "INSERT", true, ""},
		{"app_tenant", "adoption_applications", "UPDATE", true,
			"the status moves, a case gets assigned, a decision note is written — an " +
				"application is a live record, unlike the history of one"},
		{"app_tenant", "adoption_applications", "DELETE", true,
			"§5.4's retention purge and the subject-deletion path both need it; the " +
				"append-only trail of what happened is application_events (T-01-029), " +
				"not this row"},
		{"app_tenant", "adoption_applications", "TRUNCATE", false,
			"no row-level mechanism sees a truncate coming"},
		{"app_public", "adoption_applications", "SELECT", false,
			"an adoption application is the most sensitive record in this schema; the " +
				"public catalog shows animals"},
		{"app_public", "shelters", "SELECT", false,
			"no app_public policy exists for shelters yet, so a grant would be access " +
				"nothing in this phase tests"},
	} {
		var granted bool
		err := env.OwnerPool.QueryRow(ctx,
			`SELECT has_table_privilege($1, $2, $3)`, tc.role, tc.table, tc.privilege).Scan(&granted)
		if err != nil {
			t.Fatalf("reading %s's %s privilege on %s: %v", tc.role, tc.privilege, tc.table, err)
		}
		if granted != tc.want {
			t.Errorf("has_table_privilege(%s, %s, %s) = %v, want %v. %s",
				tc.role, tc.table, tc.privilege, granted, tc.want, tc.why)
		}
	}
}

// TRUNCATE is the one write no row-level mechanism can ever see, so the rule is
// universal: NO application role holds it on ANY table. That makes it an
// invariant over the catalog rather than a per-table opinion, and this test
// enumerates instead of listing.
//
// The inventory above carries a hand-written `TRUNCATE, false` row for three
// tables, which is protection for three tables. The next migration gets it only
// if its author remembers — and this project has now been bitten twice by
// exactly that shape: the grant matrix in T-01-022 and the policy inventory in
// T-01-023. A `GRANT ALL` or an `ALTER DEFAULT PRIVILEGES` (00001 declares the
// schema has neither, and this is what keeps that declaration true) would widen
// every table at once, and a hand-written list would report clean.
func TestNoApplicationRole_HoldsTruncateOnAnyTable(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	tables, err := rlstest.ReadTables(ctx, env.OwnerPool)
	if err != nil {
		t.Fatalf("reading the catalog: %v", err)
	}
	if len(tables) == 0 {
		t.Fatal("the catalog reported no tables, so this test verified nothing")
	}

	for _, table := range tables {
		for _, role := range []string{"app_tenant", "app_public"} {
			var granted bool
			if err := env.OwnerPool.QueryRow(ctx,
				`SELECT has_table_privilege($1, $2, 'TRUNCATE')`,
				role, table.Name).Scan(&granted); err != nil {
				t.Fatalf("reading %s's TRUNCATE privilege on %s: %v", role, table.Name, err)
			}
			if granted {
				t.Errorf("%s holds TRUNCATE on %s. Row level security is evaluated PER ROW "+
					"and TRUNCATE visits none — it discards the storage — so no policy, "+
					"USING clause or WITH CHECK on that table sees it coming. Only a "+
					"BEFORE TRUNCATE statement trigger could, and the grant is the layer "+
					"that should have made one unnecessary", role, table.Name)
			}
		}
	}
}

// D7: users.email is citext, so case-insensitivity is a property of the COLUMN
// TYPE rather than of caller discipline.
//
// T-01-014 asserts the BEHAVIOUR this buys — that `WHERE email = $1` finds a
// differently-cased row. This asserts the type, because between this migration
// and that task the column would otherwise be unguarded, and `text` passes every
// test in this file.
func TestUsers_EmailIsCitext(t *testing.T) {
	env := dbtest.Postgres(t)

	var typ string
	err := env.OwnerPool.QueryRow(context.Background(), `
		SELECT format_type(a.atttypid, a.atttypmod)
		FROM pg_attribute a
		WHERE a.attrelid = 'public.users'::regclass AND a.attname = 'email'`).Scan(&typ)
	if err != nil {
		t.Fatalf("reading users.email's type: %v", err)
	}
	if typ != "citext" {
		t.Errorf("users.email is %s, want citext. With text, uniqueness and lookup both "+
			"fall back to whoever remembers to normalise on every call site (D7)", typ)
	}
}

// shelters.logo_media_id and cover_media_id point at `media`, which is
// migration 00003. A constraint cannot reference a table that does not exist,
// so the foreign keys are deferred -- and this is the guard that makes the
// deferral impossible to forget.
//
// It asserts the equivalence in BOTH directions: no media table means no keys,
// and a media table means both keys. Today it passes with both halves absent.
// The day 00003 lands it goes red and names exactly what is missing, which is
// T-01-017's RED, built here.
func TestShelters_MediaColumnsGetTheirForeignKeyWhenMediaLands(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	var mediaExists bool
	if err := env.OwnerPool.QueryRow(ctx,
		`SELECT to_regclass('public.media') IS NOT NULL`).Scan(&mediaExists); err != nil {
		t.Fatalf("looking for the media table: %v", err)
	}

	rows, err := env.OwnerPool.Query(ctx, `
		SELECT a.attname
		FROM pg_constraint c
		JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = ANY (c.conkey)
		WHERE c.conrelid = 'public.shelters'::regclass
		  AND c.contype = 'f'`)
	if err != nil {
		t.Fatalf("reading shelters' foreign keys: %v", err)
	}
	defer rows.Close()

	keyed := map[string]bool{}
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			t.Fatalf("scanning a foreign key column: %v", err)
		}
		keyed[column] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading shelters' foreign keys: %v", err)
	}

	for _, column := range []string{"logo_media_id", "cover_media_id"} {
		switch {
		case mediaExists && !keyed[column]:
			t.Errorf("`media` exists but shelters.%s still has no foreign key. It was "+
				"deferred out of 00002 only because the table did not exist yet; add the "+
				"constraint in the migration that creates media", column)
		case !mediaExists && keyed[column]:
			t.Errorf("shelters.%s carries a foreign key but `media` does not exist, so it "+
				"points at something else than intended", column)
		}
	}
}
