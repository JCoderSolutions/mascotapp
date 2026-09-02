package rlstest_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gentleman/mascotapp/apps/api/internal/db"
	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
)

// D6's rule read in the other direction, and the half T-01-027 did not reach.
//
// The membership branch was made to obey it: `AND m.status = 'active'` is there
// so that REVOKING a membership takes the visibility away, not merely the access
// it otherwise grants. The applicant branch has to answer the same question, and
// its equivalent of revocation is the application going away — a withdrawal, a
// retention purge under §5.4, a subject-deletion request.
//
// If visibility outlived the application, "delete my data" would leave the
// shelter still able to read the person, which is the failure the request was
// making.
func TestApplicantVisibility_EndsWhenTheApplicationDoes(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	shelter := freshTenant(t, env)
	applicant := uuid.New()
	if err := seedMemberUser(ctx, env.OwnerPool, applicant); err != nil {
		t.Fatalf("seeding the applicant: %v", err)
	}

	application := seedApplication(t, env, shelter, seedPet(t, env, shelter), applicant)

	// Anti-vacuity: they are visible while the application stands. Without it, a
	// policy that showed nobody would pass the assertion below.
	if !userVisible(t, env, shelter, applicant) {
		t.Fatal("the applicant is not visible while their application exists, so the " +
			"disappearance below would say nothing about the application")
	}

	if err := db.WithTenant(ctx, env.TenantPool, shelter,
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
		}); err != nil {
		t.Fatalf("deleting the application: %v", err)
	}

	if userVisible(t, env, shelter, applicant) {
		t.Error("the applicant is STILL visible after their application was deleted. §5.4 " +
			"gives a subject the right to have their data removed, and a shelter that can " +
			"still read the person afterwards has not removed it. The membership branch was " +
			"fixed for exactly this shape in T-01-016; the applicant branch has to answer " +
			"the same question")
	}
}

// Assignment stays inside the shelter on the INSERT path too, not only on the
// UPDATE path T-01-027 asserted.
//
// This is not belt-and-braces. The constraint is the same one, but the two paths
// go through different code in the APPLICATION: a form that creates an
// already-assigned application never touches the update handler. A rule that
// held on only one of them would be found in production, by whichever handler
// was written second.
func TestAssignmentOnInsert_CannotLeaveTheShelter(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	shelterA, shelterB := freshTenant(t, env), freshTenant(t, env)
	petOfA := seedPet(t, env, shelterA)

	applicant, staffOfA, staffOfB := uuid.New(), uuid.New(), uuid.New()
	for _, u := range []uuid.UUID{applicant, staffOfA, staffOfB} {
		if err := seedMemberUser(ctx, env.OwnerPool, u); err != nil {
			t.Fatalf("seeding users: %v", err)
		}
	}
	if err := seedMembership(ctx, env.OwnerPool, staffOfA, shelterA); err != nil {
		t.Fatalf("seeding A's staff membership: %v", err)
	}
	if err := seedMembership(ctx, env.OwnerPool, staffOfB, shelterB); err != nil {
		t.Fatalf("seeding B's staff membership: %v", err)
	}

	create := func(assignee uuid.UUID) error {
		return db.WithTenant(ctx, env.TenantPool, shelterA,
			func(ctx context.Context, tx pgx.Tx) error {
				_, err := tx.Exec(ctx,
					`INSERT INTO adoption_applications
					     (id, shelter_id, pet_id, applicant_user_id, assigned_to_user_id)
					 VALUES ($1, $2, $3, $4, $5)`,
					uuid.New(), shelterA, petOfA, applicant, assignee)

				return err
			})
	}

	// Anti-vacuity: creating an already-assigned application is a legitimate
	// thing to do when the assignee is A's own member.
	if err := create(staffOfA); err != nil {
		t.Fatalf("shelter A could not create an application already assigned to its OWN "+
			"member, so the refusal below would be about the insert path rather than about "+
			"the assignee: %v", err)
	}

	if err := create(staffOfB); err == nil {
		t.Error("shelter A created an application already assigned to a member of shelter " +
			"B. The UPDATE path refuses this, so the constraint holds on one statement and " +
			"not the other")
	}
}

// The membership a case is assigned to cannot be deleted out from under it.
//
// Revoking a membership is an UPDATE of its `status`, not a delete — that is how
// §4.1 models it and it is what `member_visible_users` filters on. So this fires
// only on a HARD delete, and what it prevents is an application silently losing
// its owner: under CASCADE the application itself would vanish, under SET NULL it
// would quietly become unassigned and drop out of somebody's queue with nothing
// recording why.
func TestAnAssignedMembership_CannotBeDeleted(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	shelter := freshTenant(t, env)
	applicant, staff, spare := uuid.New(), uuid.New(), uuid.New()
	for _, u := range []uuid.UUID{applicant, staff, spare} {
		if err := seedMemberUser(ctx, env.OwnerPool, u); err != nil {
			t.Fatalf("seeding users: %v", err)
		}
	}
	for _, u := range []uuid.UUID{staff, spare} {
		if err := seedMembership(ctx, env.OwnerPool, u, shelter); err != nil {
			t.Fatalf("seeding a membership: %v", err)
		}
	}

	deleteMembership := func(user uuid.UUID) error {
		return db.WithTenant(ctx, env.TenantPool, shelter,
			func(ctx context.Context, tx pgx.Tx) error {
				tag, err := tx.Exec(ctx,
					`DELETE FROM memberships WHERE user_id = $1 AND shelter_id = $2`,
					user, shelter)
				if err != nil {
					return err
				}
				if tag.RowsAffected() != 1 {
					t.Errorf("deleting the membership affected %d rows rather than 1",
						tag.RowsAffected())
				}

				return nil
			})
	}

	// Anti-vacuity: a membership nothing references deletes cleanly.
	if err := deleteMembership(spare); err != nil {
		t.Fatalf("a membership nothing references could not be deleted, so the refusal "+
			"below would be about memberships rather than about the assignment: %v", err)
	}

	application := seedApplication(t, env, shelter, seedPet(t, env, shelter), applicant)
	if err := db.WithTenant(ctx, env.TenantPool, shelter,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`UPDATE adoption_applications SET assigned_to_user_id = $2 WHERE id = $1`,
				application, staff)

			return err
		}); err != nil {
		t.Fatalf("assigning the application: %v", err)
	}

	if err := deleteMembership(staff); err == nil {
		t.Error("the membership was deleted while an application was assigned to it. The " +
			"application would have lost its owner with nothing recording why — it drops " +
			"out of a queue and nobody is looking at it")
	} else if code := sqlstateOf(t, err, "deleting an assigned membership"); code !=
		sqlstateForeignKeyViolation {
		t.Errorf("deleting the membership was refused with %s rather than a foreign key "+
			"violation (%s), so ON DELETE RESTRICT is not what refused it", code,
			sqlstateForeignKeyViolation)
	}
}

// A characterization test, and the inconsistency it records is real: a shelter
// can assign a case to a member it CANNOT READ.
//
// The composite key to `memberships (user_id, shelter_id)` checks that the pair
// EXISTS. It cannot check `status`, and that is a property of PostgreSQL rather
// than an oversight — verified on 17 rather than assumed: a foreign key must
// reference a NON-PARTIAL unique constraint, so a
// `UNIQUE (user_id, shelter_id) WHERE status = 'active'` index cannot be the
// referenced key at all ("there is no unique constraint matching given keys").
//
// Meanwhile `member_visible_users` DOES filter on `status = 'active'` — it was
// given that filter in T-01-016 after Judgment Day found that any membership row
// granted read access to a user's PII. So the two layers disagree by
// construction:
//
//	assignable = the membership row exists
//	readable   = the membership row exists AND is active
//
// A case assigned to a revoked or never-accepted member sits in a queue owned by
// somebody whose name that shelter can no longer render. It is NOT a tenant leak
// — everyone involved belongs to this shelter — so the database's answer is
// incomplete rather than wrong. Closing it needs a trigger or the domain layer,
// and the assignment rules live with RBAC in Phase 02.
//
// This goes red the day somebody narrows it, and says what to do.
func TestAssignment_DoesNotYetRequireAnActiveMembership(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	shelter := freshTenant(t, env)
	applicant := uuid.New()
	if err := seedMemberUser(ctx, env.OwnerPool, applicant); err != nil {
		t.Fatalf("seeding the applicant: %v", err)
	}

	for _, status := range []string{"invited", "revoked"} {
		t.Run(status, func(t *testing.T) {
			staff := uuid.New()
			if err := seedMemberUser(ctx, env.OwnerPool, staff); err != nil {
				t.Fatalf("seeding the staff user: %v", err)
			}
			if err := seedMembershipWithStatus(
				ctx, env.OwnerPool, staff, shelter, status); err != nil {
				t.Fatalf("seeding the %s membership: %v", status, err)
			}

			// The half that is already right: this person is NOT readable. It is
			// also what makes the assignment below an inconsistency rather than
			// merely a permissive rule.
			if userVisible(t, env, shelter, staff) {
				t.Fatalf("a %s member is visible, which contradicts member_visible_users' "+
					"`m.status = 'active'` (T-01-016)", status)
			}

			application := seedApplication(
				t, env, shelter, seedPet(t, env, shelter), applicant)

			err := db.WithTenant(ctx, env.TenantPool, shelter,
				func(ctx context.Context, tx pgx.Tx) error {
					tag, err := tx.Exec(ctx,
						`UPDATE adoption_applications SET assigned_to_user_id = $2
						 WHERE id = $1`, application, staff)
					if err != nil {
						return err
					}
					if tag.RowsAffected() != 1 {
						t.Errorf("assigning touched %d rows rather than 1", tag.RowsAffected())
					}

					return nil
				})
			if err != nil {
				t.Errorf("GOOD NEWS, AND THIS TEST IS NOW WRONG: assigning a case to a %s "+
					"member was refused (%v). Something now requires an ACTIVE membership — "+
					"a trigger, or the domain layer of Phase 02's RBAC. Delete this "+
					"characterization test and assert the new rule instead", status, err)
			}
		})
	}
}
