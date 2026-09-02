package rlstest_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gentleman/mascotapp/apps/api/internal/db"
	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
)

// seedApplication records one adoption application, as the tenant that owns it.
func seedApplication(
	t *testing.T, env *dbtest.Env, shelter, pet, applicant uuid.UUID,
) uuid.UUID {
	t.Helper()

	id := uuid.New()
	err := db.WithTenant(context.Background(), env.TenantPool, shelter,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO adoption_applications
				     (id, shelter_id, pet_id, applicant_user_id)
				 VALUES ($1, $2, $3, $4)`,
				id, shelter, pet, applicant)

			return err
		})
	if err != nil {
		t.Fatalf("recording an application for pet %s: %v", pet, err)
	}

	return id
}

// D6's second permissive policy on `users`, and the reason 00002 could not carry
// it: `adoption_applications` did not exist yet.
//
// An adopter belongs to NO shelter — §4.1 gives them a magic-link account with no
// membership — so `member_visible_users` cannot see them, and a shelter that
// cannot read its own applicant cannot process the application. The applicant
// path is the carve-out, and it is a SECOND policy rather than a widened first
// one because permissive policies OR together: each one states one reason a user
// is visible, and neither can silently swallow the other.
//
// The grid below is the whole property. The row that matters most is the last
// one: a user who neither belongs to the shelter nor applied to it stays
// invisible. Without that case, `USING (true)` passes every other line here.
func TestApplicantUser_IsVisibleToTheShelterTheyAppliedTo(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	shelterA, shelterB := freshTenant(t, env), freshTenant(t, env)
	petOfA := seedPet(t, env, shelterA)

	applicant, member, stranger := uuid.New(), uuid.New(), uuid.New()
	for _, u := range []uuid.UUID{applicant, member, stranger} {
		if err := seedMemberUser(ctx, env.OwnerPool, u); err != nil {
			t.Fatalf("seeding users: %v", err)
		}
	}
	if err := seedMembership(ctx, env.OwnerPool, member, shelterA); err != nil {
		t.Fatalf("seeding the membership: %v", err)
	}

	seedApplication(t, env, shelterA, petOfA, applicant)

	for _, tc := range []struct {
		name    string
		shelter uuid.UUID
		user    uuid.UUID
		want    bool
		why     string
	}{
		{"the applicant is visible to the shelter they applied to", shelterA, applicant, true,
			"an adopter has no membership anywhere, so member_visible_users cannot see " +
				"them — and a shelter that cannot read its own applicant cannot process " +
				"the application at all"},
		{"the applicant is invisible to a shelter they did not apply to", shelterB, applicant,
			false, "this is the leak the policy's shelter_id predicate exists to prevent: " +
				"applying to one shelter must not publish the adopter's PII to every other"},
		{"a member is still visible", shelterA, member, true,
			"the two policies are PERMISSIVE and OR together; adding the applicant branch " +
				"must not narrow the membership branch"},
		{"a user who neither applied nor belongs is invisible", shelterA, stranger, false,
			"the anti-vacuity of this whole grid — `USING (true)` satisfies every line " +
				"above and fails only this one"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := userVisible(t, env, tc.shelter, tc.user); got != tc.want {
				t.Errorf("visible = %v, want %v. %s", got, tc.want, tc.why)
			}
		})
	}
}

// The spec's *"An unknown status is rejected"*, asserted in BOTH directions.
//
// One direction alone is worthless twice over: checking only the rejection would
// pass on a constraint that refuses everything, and checking only the acceptances
// would pass with no constraint at all. §4.5's set is a state machine the domain
// layer walks, so a value outside it reaches a switch with no arm for it.
//
// WHICH transitions are legal is explicitly NOT here. The database says what a
// status may be; the domain says how you get from one to the next.
func TestAdoptionApplications_StatusIsAClosedUnion(t *testing.T) {
	env := dbtest.Postgres(t)

	shelter := freshTenant(t, env)
	pet := seedPet(t, env, shelter)
	applicant := uuid.New()
	if err := seedMemberUser(context.Background(), env.OwnerPool, applicant); err != nil {
		t.Fatalf("seeding the applicant: %v", err)
	}

	insert := func(status string) error {
		return db.WithTenant(context.Background(), env.TenantPool, shelter,
			func(ctx context.Context, tx pgx.Tx) error {
				_, err := tx.Exec(ctx,
					`INSERT INTO adoption_applications
					     (id, shelter_id, pet_id, applicant_user_id, status)
					 VALUES ($1, $2, $3, $4, $5)`,
					uuid.New(), shelter, pet, applicant, status)

				return err
			})
	}

	for _, status := range []string{
		"draft", "submitted", "in_review", "interview_scheduled", "home_visit_scheduled",
		"approved", "rejected", "withdrawn", "contract_signed", "delivered", "returned",
	} {
		if err := insert(status); err != nil {
			t.Errorf("status %q is in §4.5's set and the database refused it: %v", status, err)
		}
	}

	if err := insert("under_consideration"); err == nil {
		t.Error("`under_consideration` was accepted. §4.5's status set is CLOSED because the " +
			"domain layer branches on it: a twelfth value reaches a switch with no arm for it")
	}
}

// §4.6, and the shelter-side listing it exists for: *"CREATE INDEX ON
// adoption_applications (shelter_id, status, created_at DESC)"*. Every queue a
// refuge works from is "my shelter's applications, in this status, newest
// first", and under RLS the `shelter_id` predicate is on every query whether the
// caller wrote it or not — so it belongs at the front of the key.
func TestAdoptionApplications_HasTheListingIndex(t *testing.T) {
	env := dbtest.Postgres(t)

	rows, err := env.OwnerPool.Query(context.Background(), `
		SELECT indexdef FROM pg_indexes
		WHERE schemaname = 'public' AND tablename = 'adoption_applications'`)
	if err != nil {
		t.Fatalf("reading adoption_applications' indexes: %v", err)
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
		t.Fatalf("reading adoption_applications' indexes: %v", err)
	}

	if !anyIndexContains(defs, []string{"shelter_id", "status", "created_at DESC"}) {
		t.Errorf("the application listing index is missing. §4.6 — a shelter works from a "+
			"queue of its own applications by status, newest first\ngot:\n  %v", defs)
	}
}

// D5 on this table's reference to `pets`, and the behavioural half the structural
// guard cannot make.
//
// The row carries B's OWN `shelter_id`, so the policy's WITH CHECK is satisfied
// and steps aside — a 42501 here is a FAILING result. And rejection alone is not
// the property: if a foreign pet answered differently from a nonexistent one,
// tenant B could enumerate tenant A's animals one id at a time.
func TestAdoptionApplications_CannotReferenceAnotherTenantsPet(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	tenantA, tenantB := freshTenant(t, env), freshTenant(t, env)
	petOfA := seedPet(t, env, tenantA)
	petOfB := seedPet(t, env, tenantB)

	applicant := uuid.New()
	if err := seedMemberUser(ctx, env.OwnerPool, applicant); err != nil {
		t.Fatalf("seeding the applicant: %v", err)
	}

	apply := func(pet uuid.UUID) (string, []any) {
		return `INSERT INTO adoption_applications
		            (id, shelter_id, pet_id, applicant_user_id)
		        VALUES ($1, $2, $3, $4)`,
			[]any{uuid.New(), tenantB, pet, applicant}
	}

	// Anti-vacuity: B can apply against its own pet.
	sql, args := apply(petOfB)
	if err := db.WithTenant(ctx, env.TenantPool, tenantB,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, sql, args...)

			return err
		}); err != nil {
		t.Fatalf("tenant B could not record an application against its OWN pet, so the "+
			"refusals below would prove nothing: %v", err)
	}

	sql, args = apply(petOfA)
	foreign := refusal(t, env, tenantB, sql, args,
		"tenant B recorded an application against tenant A's pet")

	sql, args = apply(uuid.New())
	absent := refusal(t, env, tenantB, sql, args,
		"tenant B recorded an application against a pet that does not exist")

	if foreign.Code == sqlstateInsufficientPrivilege {
		t.Fatalf("the cross-tenant application was refused by the POLICY (%s), not by a "+
			"foreign key. The row carries B's own shelter_id, so WITH CHECK should have "+
			"passed and the composite key should have done the refusing: %s",
			sqlstateInsufficientPrivilege, foreign.Message)
	}
	if foreign.Code != sqlstateForeignKeyViolation {
		t.Fatalf("naming tenant A's pet was refused with %s rather than a foreign key "+
			"violation (%s): %s", foreign.Code, sqlstateForeignKeyViolation, foreign.Message)
	}
	if foreign.Code != absent.Code || foreign.ConstraintName != absent.ConstraintName {
		t.Errorf("adoption_applications answers differently for another tenant's pet (%s/%s) "+
			"than for a pet that does not exist (%s/%s), so tenant B can enumerate tenant "+
			"A's animals one id at a time",
			foreign.Code, foreign.ConstraintName, absent.Code, absent.ConstraintName)
	}
}

// `assigned_to_user_id` is a composite foreign key to `memberships
// (user_id, shelter_id)`, not a plain reference to `users`, and that turns
// *"assignment stays inside the shelter"* from a rule somebody enforces in a
// handler into one the database cannot be talked out of.
//
// It works because `memberships` already carries `UNIQUE (user_id, shelter_id)`
// — the key was there for its own sake and is exactly the referenced key D5
// wants. Note what it buys beyond tidiness: without it, a shelter could assign an
// application to a person who has no relationship with it at all, and the
// assignee's name would then render in that shelter's queue.
//
// The composite key is NULLABLE and that is deliberate: an unassigned
// application is the normal state of a new one, and PostgreSQL's MATCH SIMPLE
// skips the check entirely when any column of the key is NULL.
//
// This is the FK's own assertion, made in the migration's task the way D5
// requires. The fuller grid — a revoked membership, an invited one, reassignment
// across shelters — is T-01-028's *"assignment stays inside the shelter"*.
func TestApplicationAssignment_CannotLeaveTheShelter(t *testing.T) {
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

	application := seedApplication(t, env, shelterA, petOfA, applicant)

	assign := func(to uuid.UUID) error {
		return db.WithTenant(ctx, env.TenantPool, shelterA,
			func(ctx context.Context, tx pgx.Tx) error {
				tag, err := tx.Exec(ctx,
					`UPDATE adoption_applications SET assigned_to_user_id = $2 WHERE id = $1`,
					application, to)
				if err != nil {
					return err
				}
				if tag.RowsAffected() != 1 {
					t.Errorf("assigning touched %d rows rather than 1", tag.RowsAffected())
				}

				return nil
			})
	}

	// Anti-vacuity: assigning to the shelter's own member works. Without it, a
	// column nobody can write would pass the refusal below.
	if err := assign(staffOfA); err != nil {
		t.Fatalf("shelter A could not assign its own application to its OWN member, so the "+
			"refusal below proves nothing: %v", err)
	}

	if err := assign(staffOfB); err == nil {
		t.Error("shelter A assigned its application to a member of shelter B. The assignee " +
			"renders in A's queue, so this publishes B's staff into A — and it is the " +
			"composite key to memberships (user_id, shelter_id) that has to refuse it, " +
			"because no policy sees a value the tenant writes into its own row")
	}
}

// A characterization test: it records what the applicant policy costs TODAY, so
// the day the fix lands the change is visible rather than silent.
//
// D6's own rule, written when Judgment Day found it on `memberships`: **when a
// policy on table P derives visibility through bridge table B, WRITE permission
// on B is READ permission on P.** The bridge here is `adoption_applications` and
// `app_tenant` holds INSERT on it, so a tenant that already knows a user's uuid
// can mint read access to that user's PII by inserting an application naming
// them — no membership, no consent, no application the person ever filled.
//
// Why it was not closed with a predicate the way `memberships` was: there, the
// filter was `m.status = 'active'`, and it worked because an INVITATION is a
// state the business already distinguished from a relationship. Here the shelter
// writes the status too, so `status <> 'draft'` buys nothing — a forged row can
// simply say `submitted`. There is no state at this layer that the attacker does
// not also control.
//
// The real fix is column-level grants, which is Judgment Day's finding B1, which
// the user deferred to Phase 02/03 because which columns a tenant may write
// depends on endpoints Phase 03 has not written. This test is the marker on that
// debt: when B1 is paid, this goes red and says what to do.
func TestApplicantPolicy_IsAsWideAsWritingAnApplication(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	shelter := freshTenant(t, env)
	pet := seedPet(t, env, shelter)

	// A person with no relationship to this shelter at all.
	outsider := uuid.New()
	if err := seedMemberUser(ctx, env.OwnerPool, outsider); err != nil {
		t.Fatalf("seeding the outsider: %v", err)
	}

	// Anti-vacuity: before the forged row, the outsider is invisible. Without
	// this, a policy that leaked everything would produce the same final read
	// and the test would report a narrow exposure as a wide one.
	if userVisible(t, env, shelter, outsider) {
		t.Fatal("a user with no membership and no application is already visible, so this " +
			"test cannot attribute the visibility below to the forged application")
	}

	seedApplication(t, env, shelter, pet, outsider)

	if !userVisible(t, env, shelter, outsider) {
		t.Error("GOOD NEWS, AND THIS TEST IS NOW WRONG: a shelter can no longer read a user " +
			"by inserting an application naming them. Something narrowed the write path — " +
			"column-level grants (Judgment Day finding B1), a trigger, or a policy " +
			"predicate. Delete this characterization test and assert the new rule instead")
	}
}

// The cascade that would let a shelter erase every application it ever received
// by deleting the animal, and the reason `ON DELETE RESTRICT` is on that key.
//
// Mutation found it: switching the pet reference to `ON DELETE CASCADE` broke no
// test. `app_tenant` holds DELETE on `pets`, so nothing above the database stops
// it, and the applications would go silently — no error, no trace, because the
// trail of what happened to them lives in tables that would cascade with them.
//
// Same shape as `pet_status_history` in T-01-020 and the draft version in
// T-01-024. Three times now the hole was not in the table being protected but in
// what happens to it when its PARENT goes.
func TestApplications_CannotBeErasedByDeletingTheirPet(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	shelter := freshTenant(t, env)
	applicant := uuid.New()
	if err := seedMemberUser(ctx, env.OwnerPool, applicant); err != nil {
		t.Fatalf("seeding the applicant: %v", err)
	}

	deletePet := func(pet uuid.UUID) error {
		return db.WithTenant(ctx, env.TenantPool, shelter,
			func(ctx context.Context, tx pgx.Tx) error {
				tag, err := tx.Exec(ctx, `DELETE FROM pets WHERE id = $1`, pet)
				if err != nil {
					return err
				}
				if tag.RowsAffected() != 1 {
					t.Errorf("deleting the pet affected %d rows rather than 1. A statement "+
						"that reached no row would look exactly like a refusal here",
						tag.RowsAffected())
				}

				return nil
			})
	}

	// Anti-vacuity: a pet with NO application deletes cleanly. Without it, a
	// schema where pets could never be deleted at all would pass below while
	// saying nothing about applications.
	if err := deletePet(seedPet(t, env, shelter)); err != nil {
		t.Fatalf("a pet with no applications could not be deleted, so the refusal below "+
			"would be about pets rather than about what hangs off them: %v", err)
	}

	pet := seedPet(t, env, shelter)
	seedApplication(t, env, shelter, pet, applicant)

	if err := deletePet(pet); err == nil {
		t.Fatal("the pet was deleted and took its adoption applications with it. A shelter " +
			"holds DELETE on pets, so a cascade here is a one-statement erasure of every " +
			"application it ever received — and an animal leaves the catalog by changing " +
			"status, never by disappearing (LT-5)")
	} else if code := sqlstateOf(t, err, "deleting a pet that has applications"); code !=
		sqlstateForeignKeyViolation {
		t.Errorf("deleting the pet was refused with %s rather than a foreign key violation "+
			"(%s), so ON DELETE RESTRICT is not what refused it", code,
			sqlstateForeignKeyViolation)
	}
}

// A structural assertion for a layer that BEHAVIOUR cannot see, and the reason
// it is written this way is the finding itself.
//
// Mutation removed the `a.shelter_id = <current shelter>` predicate from the
// applicant policy and the entire suite stayed green — including the case above
// that asserts tenant B cannot see tenant A's applicant. Investigated rather
// than accepted, and the explanation is T-01-020's mutant M13 restated: a
// policy's `EXISTS` subquery is ITSELF filtered by the referenced table's row
// security. The subquery reads `adoption_applications`, which carries
// `tenant_isolation`, so it already returns nothing for another shelter's rows.
// The predicate is redundant GIVEN that policy.
//
// Which is exactly why it stays, and why it is pinned here. "Redundant given
// another table's policy" is not the same as "not needed": it makes this
// policy's isolation depend on a policy on a DIFFERENT table, so the day
// `adoption_applications` changes shape — a second permissive policy, a role
// that is not subject to it, a `FOR SELECT USING (true)` added for some
// reporting path — the leak arrives here and nothing behavioural sees it coming.
// `member_visible_users` names `m.shelter_id` for the same reason, and this is
// the assertion that keeps both honest.
func TestApplicantPolicy_ScopesItselfRatherThanBorrowingTheChildsPolicy(t *testing.T) {
	env := dbtest.Postgres(t)

	if !policyMentions(t, env, "users", "applicant_visible_users", "app.shelter_id") {
		t.Error("applicant_visible_users does not name `app.shelter_id` in its own USING " +
			"clause, so its cross-tenant isolation is borrowed entirely from " +
			"adoption_applications' policy. Behaviour cannot tell the difference today — " +
			"a policy's EXISTS subquery is filtered by the referenced table's RLS — and " +
			"that is the point: the day that other policy widens, this one widens with it " +
			"and no test notices")
	}

	// The same claim for the branch that was already there, so the pair is
	// asserted by one rule rather than one of them being covered by accident.
	if !policyMentions(t, env, "users", "member_visible_users", "app.shelter_id") {
		t.Error("member_visible_users does not name `app.shelter_id` in its own USING " +
			"clause — same failure as above, on the branch D6 wrote first")
	}
}
