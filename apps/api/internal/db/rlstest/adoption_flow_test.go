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

	// Since 00015 (P2-D5) app_tenant holds NO DELETE grant on memberships, so
	// running this probe as the tenant now gets 42501 before the foreign key is
	// ever consulted — a privilege answering for a constraint, which would make
	// this test green while asserting nothing about ON DELETE RESTRICT.
	//
	// The constraint still matters and still has to be proven: it is what stops
	// the deletion whenever a role DOES hold the privilege — an operator today,
	// and app_tenant again the day a migration widens the grant back. So the
	// probe moved to the owner, the role that still has it, and now measures the
	// constraint instead of the grant. The stronger claim about the tenant is
	// asserted separately, below.
	deleteMembership := func(user uuid.UUID) error {
		tag, err := env.OwnerPool.Exec(ctx,
			`DELETE FROM memberships WHERE user_id = $1 AND shelter_id = $2`, user, shelter)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			t.Errorf("deleting the membership affected %d rows rather than 1",
				tag.RowsAffected())
		}

		return nil
	}

	// Defence in depth, and the outer layer is newer than this test: whatever
	// the foreign key decides, app_tenant cannot reach a DELETE on memberships
	// at all. If this ever stops being true, 00015's grant has been widened and
	// the constraint below is once again the ONLY thing in the way.
	if err := db.WithTenant(ctx, env.TenantPool, shelter,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`DELETE FROM memberships WHERE user_id = $1 AND shelter_id = $2`, spare, shelter)

			return err
		}); err == nil {
		t.Error("app_tenant deleted a membership. 00015 revokes DELETE on memberships " +
			"(P2-D5) because revocation is status = 'revoked', which keeps the trail")
	} else if code := sqlstateOf(t, err, "app_tenant deleting a membership"); code !=
		sqlstateInsufficientPrivilege {
		t.Errorf("app_tenant's DELETE was refused with %s rather than the missing grant "+
			"(%s), so what refused it is unproven", code, sqlstateInsufficientPrivilege)
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

// The rule `00016_assignee_active_membership` installs, and the test that
// REPLACED the characterization test pinning its absence.
//
// Until T-02-007 this file carried `TestAssignment_DoesNotYetRequireAnActiveMembership`,
// which recorded that a shelter could assign a case to a member it CANNOT READ.
// Two layers disagreed by construction:
//
//	assignable = the membership row exists
//	readable   = the membership row exists AND is active
//
// The composite foreign key to `memberships (user_id, shelter_id)` checks that
// the pair EXISTS and cannot check `status` — a property of PostgreSQL rather
// than an oversight, verified on 17: a foreign key must reference a NON-PARTIAL
// unique constraint, so `UNIQUE (user_id, shelter_id) WHERE status = 'active'`
// cannot be the referenced key at all. Meanwhile `member_visible_users` DOES
// filter on `status = 'active'`, given that filter in T-01-016 after Judgment
// Day found that any membership row granted read access to a user's PII.
//
// A case assigned to a revoked or never-accepted member sat in a queue owned by
// somebody whose name that shelter could no longer render.
//
// P2-D7 closes it with a `BEFORE INSERT OR UPDATE OF assigned_to_user_id` row
// trigger. A trigger and not a policy, for ADR-0010's reason: a trigger is NOT
// bypassed by a `BYPASSRLS` role. The subquery runs as the invoking role, so
// under `app_tenant` it is filtered by `memberships`' own tenant policy — the
// correct scope, not a limitation.
//
// The old test and this one changed hands in ONE commit, with the migration.
// Landing them apart would leave the suite red between two commits with no code
// change to explain why.
func TestAssignment_RequiresAnActiveMembership(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	const sqlstateCheckViolation = "23514"

	shelter := freshTenant(t, env)
	applicant := uuid.New()
	if err := seedMemberUser(ctx, env.OwnerPool, applicant); err != nil {
		t.Fatalf("seeding the applicant: %v", err)
	}

	// assign returns the error of assigning `application` to `staff`, or nil.
	assign := func(application, staff uuid.UUID) error {
		return db.WithTenant(ctx, env.TenantPool, shelter,
			func(ctx context.Context, tx pgx.Tx) error {
				tag, err := tx.Exec(ctx,
					`UPDATE adoption_applications SET assigned_to_user_id = $2 WHERE id = $1`,
					application, staff)
				if err != nil {
					return err
				}
				if tag.RowsAffected() != 1 {
					t.Errorf("assigning touched %d rows rather than 1. A statement that "+
						"reached no row looks exactly like a refusal here", tag.RowsAffected())
				}

				return nil
			})
	}

	// staffWithStatus seeds a fresh user holding one membership in this shelter.
	staffWithStatus := func(t *testing.T, status string) uuid.UUID {
		t.Helper()

		staff := uuid.New()
		if err := seedMemberUser(ctx, env.OwnerPool, staff); err != nil {
			t.Fatalf("seeding the staff user: %v", err)
		}
		if err := seedMembershipWithStatus(ctx, env.OwnerPool, staff, shelter, status); err != nil {
			t.Fatalf("seeding the %s membership: %v", status, err)
		}

		return staff
	}

	for _, status := range []string{"invited", "revoked"} {
		t.Run("update_to_a_"+status+"_member_is_refused", func(t *testing.T) {
			staff := staffWithStatus(t, status)

			// The half that was already right, kept from the test this replaces:
			// this person is not readable. It is what made the old behaviour an
			// inconsistency rather than merely a permissive rule.
			if userVisible(t, env, shelter, staff) {
				t.Fatalf("a %s member is visible, which contradicts member_visible_users' "+
					"`m.status = 'active'` (T-01-016)", status)
			}

			application := seedApplication(t, env, shelter, seedPet(t, env, shelter), applicant)

			err := assign(application, staff)
			if err == nil {
				t.Fatalf("a case was assigned to a %s member. It lands in a queue owned by "+
					"somebody this shelter cannot even render the name of", status)
			}
			if code := sqlstateOf(t, err, "assigning to a "+status+" member"); code !=
				sqlstateCheckViolation {
				t.Errorf("the assignment was refused with %s rather than the trigger's %s, "+
					"so what refused it is unproven -- a foreign key or a policy would "+
					"refuse a DIFFERENT set of assignees than this rule does",
					code, sqlstateCheckViolation)
			}
		})
	}

	// Anti-vacuity. Without it every case above is satisfied by a trigger that
	// refuses EVERY assignment, which would break the feature rather than narrow
	// it -- and would look identical from the refusals alone.
	t.Run("update_to_an_active_member_succeeds", func(t *testing.T) {
		staff := staffWithStatus(t, "active")
		application := seedApplication(t, env, shelter, seedPet(t, env, shelter), applicant)

		if err := assign(application, staff); err != nil {
			t.Fatalf("assigning to an ACTIVE member was refused: %v. The trigger did not "+
				"narrow the rule, it broke it -- no case can be assigned to anyone", err)
		}
	})

	// The trigger fires on INSERT too, and that arm needs its own case: an
	// application can be created already assigned, so a trigger covering only
	// UPDATE would leave the same hole reachable through a different statement.
	t.Run("insert_already_assigned_to_a_revoked_member_is_refused", func(t *testing.T) {
		staff := staffWithStatus(t, "revoked")
		pet := seedPet(t, env, shelter)

		err := db.WithTenant(ctx, env.TenantPool, shelter,
			func(ctx context.Context, tx pgx.Tx) error {
				_, err := tx.Exec(ctx,
					`INSERT INTO adoption_applications
					     (id, shelter_id, pet_id, applicant_user_id, assigned_to_user_id)
					 VALUES ($1, $2, $3, $4, $5)`,
					uuid.New(), shelter, pet, applicant, staff)

				return err
			})
		if err == nil {
			t.Fatal("an application was CREATED already assigned to a revoked member. The " +
				"trigger covers UPDATE but not INSERT, so the rule is reachable around it")
		}
		if code := sqlstateOf(t, err, "inserting an already-assigned application"); code !=
			sqlstateCheckViolation {
			t.Errorf("the insert was refused with %s rather than the trigger's %s",
				code, sqlstateCheckViolation)
		}
	})

	// The `WHEN (NEW.assigned_to_user_id IS NOT NULL)` guard, asserted rather
	// than assumed. Unassigning is how a case goes back to the queue; a trigger
	// without the guard raises on the NULL because no membership row matches it,
	// and the feature would be dead with no test saying so.
	t.Run("unassigning_is_still_allowed", func(t *testing.T) {
		staff := staffWithStatus(t, "active")
		application := seedApplication(t, env, shelter, seedPet(t, env, shelter), applicant)

		if err := assign(application, staff); err != nil {
			t.Fatalf("assigning to an active member: %v", err)
		}

		if err := db.WithTenant(ctx, env.TenantPool, shelter,
			func(ctx context.Context, tx pgx.Tx) error {
				_, err := tx.Exec(ctx,
					`UPDATE adoption_applications SET assigned_to_user_id = NULL WHERE id = $1`,
					application)

				return err
			}); err != nil {
			t.Fatalf("unassigning was refused: %v. The trigger's WHEN guard is missing, so "+
				"it fires on a NULL assignee and no case can ever return to the queue", err)
		}
	})
}
