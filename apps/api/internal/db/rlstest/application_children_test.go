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

// seedEvent appends one event to an application's timeline.
func seedEvent(t *testing.T, env *dbtest.Env, shelter, application uuid.UUID) uuid.UUID {
	t.Helper()

	id := uuid.New()
	err := db.WithTenant(context.Background(), env.TenantPool, shelter,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO application_events (id, shelter_id, application_id, type)
				 VALUES ($1, $2, $3, 'status_changed')`,
				id, shelter, application)

			return err
		})
	if err != nil {
		t.Fatalf("appending an event to application %s: %v", application, err)
	}

	return id
}

// seedNote writes one internal note on an application.
func seedNote(t *testing.T, env *dbtest.Env, shelter, application uuid.UUID) uuid.UUID {
	t.Helper()

	id := uuid.New()
	err := db.WithTenant(context.Background(), env.TenantPool, shelter,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO application_notes (id, shelter_id, application_id, body)
				 VALUES ($1, $2, $3, 'Called the applicant')`,
				id, shelter, application)

			return err
		})
	if err != nil {
		t.Fatalf("writing a note on application %s: %v", application, err)
	}

	return id
}

// applicationChildren is each child's minimal insert, parameterised by exactly
// the pair its composite foreign key checks.
var applicationChildren = []struct {
	table string
	build func(shelter, application uuid.UUID) (string, []any)
}{
	{
		table: "application_events",
		build: func(shelter, application uuid.UUID) (string, []any) {
			return `INSERT INTO application_events (id, shelter_id, application_id, type)
			        VALUES ($1, $2, $3, 'status_changed')`,
				[]any{uuid.New(), shelter, application}
		},
	},
	{
		table: "application_notes",
		build: func(shelter, application uuid.UUID) (string, []any) {
			return `INSERT INTO application_notes (id, shelter_id, application_id, body)
			        VALUES ($1, $2, $3, 'note')`,
				[]any{uuid.New(), shelter, application}
		},
	},
}

// D5 on both children, and the same oracle-closing shape the pet children got in
// T-01-021: the row carries B's OWN `shelter_id`, so the policy steps aside and
// the composite key is what must refuse it — and it must refuse a FOREIGN
// application exactly as it refuses one that does not exist.
//
// The stakes are higher here than on the pet children. Distinguishing "that
// application belongs to another shelter" from "that application does not exist"
// is a probe over another refuge's adoption cases.
func TestApplicationChildren_CannotReferenceAnotherTenantsApplication(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	tenantA, tenantB := freshTenant(t, env), freshTenant(t, env)

	applicant := uuid.New()
	if err := seedMemberUser(ctx, env.OwnerPool, applicant); err != nil {
		t.Fatalf("seeding the applicant: %v", err)
	}
	applicationOfA := seedApplication(t, env, tenantA, seedPet(t, env, tenantA), applicant)
	applicationOfB := seedApplication(t, env, tenantB, seedPet(t, env, tenantB), applicant)

	for _, tc := range applicationChildren {
		t.Run(tc.table, func(t *testing.T) {
			// Anti-vacuity: B can write this row against its OWN application, so
			// every other constraint on it is satisfiable and a refusal below can
			// only be about the parent.
			sql, args := tc.build(tenantB, applicationOfB)
			if err := db.WithTenant(ctx, env.TenantPool, tenantB,
				func(ctx context.Context, tx pgx.Tx) error {
					_, err := tx.Exec(ctx, sql, args...)

					return err
				}); err != nil {
				t.Fatalf("tenant B could not write a row of %s against its OWN application, "+
					"so both refusals below would prove nothing: %v", tc.table, err)
			}

			sql, args = tc.build(tenantB, applicationOfA)
			foreign := refusal(t, env, tenantB, sql, args,
				"tenant B wrote a row of "+tc.table+" against tenant A's application")

			sql, args = tc.build(tenantB, uuid.New())
			absent := refusal(t, env, tenantB, sql, args,
				"tenant B wrote a row of "+tc.table+" against an application that does not exist")

			if foreign.Code == sqlstateInsufficientPrivilege {
				t.Fatalf("the cross-tenant insert into %s was refused by the POLICY (%s), "+
					"not by a foreign key. The row carries B's own shelter_id, so WITH "+
					"CHECK should have passed: %s",
					tc.table, sqlstateInsufficientPrivilege, foreign.Message)
			}
			if foreign.Code != sqlstateForeignKeyViolation {
				t.Fatalf("naming tenant A's application in %s was refused with %s rather "+
					"than a foreign key violation (%s): %s",
					tc.table, foreign.Code, sqlstateForeignKeyViolation, foreign.Message)
			}
			if foreign.Code != absent.Code || foreign.ConstraintName != absent.ConstraintName {
				t.Errorf("%s answers differently for another tenant's application (%s/%s) "+
					"than for one that does not exist (%s/%s), so tenant B can enumerate "+
					"tenant A's adoption cases one id at a time",
					tc.table,
					foreign.Code, foreign.ConstraintName, absent.Code, absent.ConstraintName)
			}
		})
	}
}

// The obligation T-01-027 wrote down, paid here — and it is the reason
// `form_submissions.application_id` was allowed to CASCADE.
//
// §5.4's retention purge deletes a rejected application, and 00009 made the
// answers go with it deliberately: the child there is the PERSONAL DATA and
// leaving it behind is the privacy failure. That choice is only safe while the
// EVIDENCE lives somewhere that does NOT cascade. These two tables are that
// somewhere.
//
// With a cascading key, purging a solicitud would erase the record that it ever
// existed — who reviewed it, when it was rejected, what was said — and the audit
// trail would disappear through a door nobody was watching, exactly as
// `pet_status_history` would have (T-01-020).
func TestApplicationChildren_CannotBeErasedByDeletingTheirApplication(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	shelter := freshTenant(t, env)
	applicant := uuid.New()
	if err := seedMemberUser(ctx, env.OwnerPool, applicant); err != nil {
		t.Fatalf("seeding the applicant: %v", err)
	}

	deleteApplication := func(application uuid.UUID) error {
		return db.WithTenant(ctx, env.TenantPool, shelter,
			func(ctx context.Context, tx pgx.Tx) error {
				tag, err := tx.Exec(ctx,
					`DELETE FROM adoption_applications WHERE id = $1`, application)
				if err != nil {
					return err
				}
				if tag.RowsAffected() != 1 {
					t.Errorf("deleting the application affected %d rows rather than 1",
						tag.RowsAffected())
				}

				return nil
			})
	}

	// Anti-vacuity: an application with no children deletes cleanly. §5.4's
	// retention purge has to work, so "nothing can ever be deleted" would be a
	// failing outcome here, not a passing one.
	bare := seedApplication(t, env, shelter, seedPet(t, env, shelter), applicant)
	if err := deleteApplication(bare); err != nil {
		t.Fatalf("an application with no events or notes could not be deleted, so §5.4's "+
			"retention purge cannot run at all and the refusals below say nothing about "+
			"the children: %v", err)
	}

	for _, tc := range []struct {
		name string
		seed func(shelter, application uuid.UUID) uuid.UUID
	}{
		{"application_events", func(s, a uuid.UUID) uuid.UUID { return seedEvent(t, env, s, a) }},
		{"application_notes", func(s, a uuid.UUID) uuid.UUID { return seedNote(t, env, s, a) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			application := seedApplication(t, env, shelter, seedPet(t, env, shelter), applicant)
			tc.seed(shelter, application)

			err := deleteApplication(application)
			if err == nil {
				t.Fatalf("the application was deleted and took its %s with it. That is the "+
					"trail of what happened to a person's adoption request — and it is what "+
					"makes 00009's CASCADE on form_submissions.application_id safe. With "+
					"both cascading, a retention purge erases the evidence that the "+
					"solicitud ever existed", tc.name)
			}
			if code := sqlstateOf(t, err, "deleting an application that has "+tc.name); code !=
				sqlstateForeignKeyViolation {
				t.Errorf("the delete was refused with %s rather than a foreign key violation "+
					"(%s), so ON DELETE RESTRICT is not what refused it",
					code, sqlstateForeignKeyViolation)
			}
		})
	}
}

// The grant stops `app_tenant`. This stops everyone else.
//
// It runs as the OWNER, which in this container is a superuser: the role that
// holds every privilege and skips every policy. Neither the revoked grant nor
// the absent UPDATE/DELETE policy survives a role with BYPASSRLS, and on Neon
// that is not hypothetical — `neon_superuser` carries it. RLS is bypassed by
// such roles; TRIGGERS ARE NOT. So if the timeline is still immutable here, the
// trigger is what is doing the work, because nothing else could be.
//
// It also turns a SILENT zero-row result — which calling code readily misreads
// as success — into a loud error, which is the spec's own reason for the layer.
func TestApplicationEvents_AreAppendOnlyEvenForARoleThatBypassesRowSecurity(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	shelter := freshTenant(t, env)
	applicant := uuid.New()
	if err := seedMemberUser(ctx, env.OwnerPool, applicant); err != nil {
		t.Fatalf("seeding the applicant: %v", err)
	}
	application := seedApplication(t, env, shelter, seedPet(t, env, shelter), applicant)
	event := seedEvent(t, env, shelter, application)

	// Anti-vacuity: the owner CAN append. Without this, a table nobody could
	// write to at all would pass every case below and read as append-only.
	if _, err := env.OwnerPool.Exec(ctx,
		`INSERT INTO application_events (id, shelter_id, application_id, type)
		 VALUES ($1, $2, $3, 'assigned')`,
		uuid.New(), shelter, application); err != nil {
		t.Fatalf("the owner cannot APPEND to application_events, so the refusals below "+
			"would prove the table is unusable rather than append-only: %v", err)
	}

	for _, tc := range []struct {
		name      string
		statement string
	}{
		{"update", `UPDATE application_events SET type = 'rewritten' WHERE id = $1`},
		{"delete", `DELETE FROM application_events WHERE id = $1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := env.OwnerPool.Exec(ctx, tc.statement, event)
			if err == nil {
				t.Fatalf("a superuser %sd a row of application_events. The grant and the "+
					"missing policy both stop app_tenant and neither stops a BYPASSRLS "+
					"role; only a trigger does, and there is none", tc.name)
			}
			assertRaisedByTheTrigger(t, err, tc.name)
		})
	}

	// TRUNCATE removes every row without visiting any, so USING and WITH CHECK
	// are never consulted and a FOR EACH ROW trigger never fires. Only a
	// statement-level trigger reaches it.
	t.Run("truncate", func(t *testing.T) {
		_, err := env.OwnerPool.Exec(ctx, `TRUNCATE application_events`)
		if err == nil {
			t.Fatal("application_events was TRUNCATEd. A row-level policy cannot see a " +
				"truncate at all, so the BEFORE TRUNCATE statement trigger is the only " +
				"thing that could have stopped it")
		}
		assertRaisedByTheTrigger(t, err, "truncate")
	})

	// And the spec asks for it explicitly: the row is unchanged. A refusal that
	// still wrote would be the worst outcome of all.
	var kind string
	if err := env.OwnerPool.QueryRow(ctx,
		`SELECT type FROM application_events WHERE id = $1`, event).Scan(&kind); err != nil {
		t.Fatalf("reading the event back: %v", err)
	}
	if kind != "status_changed" {
		t.Errorf("the event's type is now %q. Every statement above was refused and one of "+
			"them wrote anyway", kind)
	}
}

// assertRaisedByTheTrigger checks the refusal came from the plpgsql RAISE rather
// than from a grant. `P0001` is deliberately distinct from the grant's `42501`
// precisely so a test can tell which of the three layers fired — asserting only
// that SOMETHING refused would let one layer cover for the other two.
func assertRaisedByTheTrigger(t *testing.T, err error, what string) {
	t.Helper()

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != sqlstateRaiseException {
		t.Fatalf("the %s was refused, but not by the append-only trigger (%s), so what "+
			"refused it is unproven: %v", what, sqlstateRaiseException, err)
	}
}

// The contrast, and it is deliberate rather than an omission.
//
// `application_notes` is NOT append-only. A note is a person writing a sentence
// about a case — "called, no answer" — and people make typos and change their
// minds. §4.5 lists `application_events` and `audit_log` as the trail; a note is
// working memory, not evidence.
//
// This is also the anti-vacuity for the whole append-only claim above. If BOTH
// tables refused every write, "application_events is append-only" would be
// indistinguishable from "this migration produced two read-only tables".
func TestApplicationNotes_AreEditableUnlikeTheEventTrail(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	shelter := freshTenant(t, env)
	applicant := uuid.New()
	if err := seedMemberUser(ctx, env.OwnerPool, applicant); err != nil {
		t.Fatalf("seeding the applicant: %v", err)
	}
	application := seedApplication(t, env, shelter, seedPet(t, env, shelter), applicant)
	note := seedNote(t, env, shelter, application)

	// RowsAffected, not the absence of an error: a BEFORE trigger returning NULL
	// cancels its statement SILENTLY, so `err == nil` would report a note as
	// editable while every change was quietly discarded (T-01-023).
	for _, tc := range []struct {
		name      string
		statement string
	}{
		{"edit", `UPDATE application_notes SET body = 'Called again' WHERE id = $1`},
		{"delete", `DELETE FROM application_notes WHERE id = $1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := db.WithTenant(ctx, env.TenantPool, shelter,
				func(ctx context.Context, tx pgx.Tx) error {
					tag, err := tx.Exec(ctx, tc.statement, note)
					if err != nil {
						return err
					}
					if tag.RowsAffected() != 1 {
						t.Errorf("the %s reported no error and touched %d rows. A BEFORE "+
							"trigger returning NULL cancels the statement without saying so",
							tc.name, tag.RowsAffected())
					}

					return nil
				}); err != nil {
				t.Fatalf("a shelter could not %s its own note. Notes are working memory, "+
					"not the audit trail — that is application_events: %v", tc.name, err)
			}
		})
	}
}

// The spec's *"An unknown visibility is rejected"*, in both directions.
//
// This phase STORES the value; enforcing who may read which visibility belongs
// to a later phase. But the column has to be a closed set from the start,
// because an `internal` note that a later phase renders as `shared` — through a
// third value nobody wrote a branch for — shows an adopter what the shelter said
// about them.
func TestApplicationNotes_VisibilityIsAClosedUnion(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	shelter := freshTenant(t, env)
	applicant := uuid.New()
	if err := seedMemberUser(ctx, env.OwnerPool, applicant); err != nil {
		t.Fatalf("seeding the applicant: %v", err)
	}
	application := seedApplication(t, env, shelter, seedPet(t, env, shelter), applicant)

	insert := func(visibility string) error {
		return db.WithTenant(ctx, env.TenantPool, shelter,
			func(ctx context.Context, tx pgx.Tx) error {
				_, err := tx.Exec(ctx,
					`INSERT INTO application_notes
					     (id, shelter_id, application_id, body, visibility)
					 VALUES ($1, $2, $3, 'note', $4)`,
					uuid.New(), shelter, application, visibility)

				return err
			})
	}

	for _, visibility := range []string{"internal", "shared"} {
		if err := insert(visibility); err != nil {
			t.Errorf("visibility %q is in the declared set and the database refused it: %v",
				visibility, err)
		}
	}

	if err := insert("public"); err == nil {
		t.Error("`public` was accepted as a note visibility. The set is closed because a " +
			"later phase branches on it to decide who may read the note: a third value " +
			"reaches a switch with no arm for it, and the fallback decides whether an " +
			"adopter sees what the shelter wrote about them")
	}
}
