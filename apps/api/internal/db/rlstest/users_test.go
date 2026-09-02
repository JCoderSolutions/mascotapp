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

// `users` is the one table in this schema with no shelter_id, so it is the one
// table whose isolation the A/B runner cannot express. These are its assertions.
//
// Everything that reads runs as app_tenant through WithTenant. The owner appears
// only to arrange rows, and it has to: app_tenant holds SELECT and nothing else
// on `users` this phase, and the arrangements below deliberately include
// memberships in shelters the reader is NOT scoped to — which app_tenant could
// never create, since its own policy would refuse them.

// userVisible reports whether a tenant session scoped to `shelter` can see the
// user with this id. Presence by id rather than a row count on purpose: the
// container is shared, and neighbouring tests leave users behind.
func userVisible(t *testing.T, env *dbtest.Env, shelter, user uuid.UUID) bool {
	t.Helper()

	var found int
	err := db.WithTenant(context.Background(), env.TenantPool, shelter,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				`SELECT count(*) FROM users WHERE id = $1`, user).Scan(&found)
		})
	if err != nil {
		t.Fatalf("reading users under tenant scope %s: %v", shelter, err)
	}

	return found == 1
}

// D6's carve-out, asserted in both directions and for both tenants.
//
// One direction is not enough, and neither is one tenant. A policy of
// `USING (false)` hides everything and passes "V is invisible to B"; a policy of
// `USING (true)` shows everything and passes "U is visible to A". Only the full
// grid fails both.
//
// The third user is the one the spec does not spell out and the product needs
// most: an adopter with no membership anywhere. If `users` were visible to any
// authenticated tenant, every adopter's row would be readable by every shelter
// on the platform.
//
// What this grid CANNOT distinguish, and it is worth knowing: deleting the
// tenant scope from the users policy leaves it green. Not because the grid is
// weak, but because the policy's EXISTS subquery is itself filtered by
// memberships' policy, so the scope is applied twice. Mutation proved that on
// 2026-08-30 and the migration records it. The consequence for whoever reads
// this next: users' isolation is a CONJUNCTION, and half of it is asserted by
// the memberships case in the A/B suite, not here.
//
// The invited and revoked cases below are the other half of the grid deleting
// the STATUS predicate leaves green: without `AND m.status = 'active'`, any
// membership row -- however it got there, whatever its status -- makes the
// user visible. app_tenant holds INSERT and UPDATE on memberships, so that gap
// would let a tenant mint visibility of an arbitrary user by inserting an
// `invited` row for them, and revoking it would not take the visibility back.
// Confirmed live against PostgreSQL 17 on 2026-08-30 (see the migration's
// comment on member_visible_users).
func TestUsers_AreVisibleOnlyThroughAMembershipInTheCurrentShelter(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	memberOfA, memberOfB, unaffiliated := uuid.New(), uuid.New(), uuid.New()
	invitedToA, revokedFromA := uuid.New(), uuid.New()

	if err := seedShelters(ctx, env.OwnerPool, env.ShelterA, env.ShelterB); err != nil {
		t.Fatalf("seeding shelters: %v", err)
	}
	for _, u := range []uuid.UUID{memberOfA, memberOfB, unaffiliated, invitedToA, revokedFromA} {
		if err := seedMemberUser(ctx, env.OwnerPool, u); err != nil {
			t.Fatalf("seeding users: %v", err)
		}
	}
	if err := seedMembership(ctx, env.OwnerPool, memberOfA, env.ShelterA); err != nil {
		t.Fatalf("seeding memberships: %v", err)
	}
	if err := seedMembership(ctx, env.OwnerPool, memberOfB, env.ShelterB); err != nil {
		t.Fatalf("seeding memberships: %v", err)
	}
	if err := seedMembershipWithStatus(ctx, env.OwnerPool, invitedToA, env.ShelterA, "invited"); err != nil {
		t.Fatalf("seeding the invited membership: %v", err)
	}
	if err := seedMembershipWithStatus(ctx, env.OwnerPool, revokedFromA, env.ShelterA, "revoked"); err != nil {
		t.Fatalf("seeding the revoked membership: %v", err)
	}

	for _, tc := range []struct {
		name    string
		shelter uuid.UUID
		user    uuid.UUID
		want    bool
		why     string
	}{
		{"a member of the current shelter is visible", env.ShelterA, memberOfA, true,
			"the shelter cannot administer a member it cannot read"},
		{"a member of another shelter is invisible", env.ShelterA, memberOfB, false,
			"this is the cross-tenant leak the policy exists to prevent"},
		{"the same grid from the other tenant", env.ShelterB, memberOfB, true, ""},
		{"and its other half", env.ShelterB, memberOfA, false, ""},
		{"a user with no membership anywhere is invisible to A", env.ShelterA, unaffiliated,
			false, "an adopter belongs to no shelter; a tenant must not be able to read them"},
		{"a user with no membership anywhere is invisible to B", env.ShelterB, unaffiliated,
			false, "same"},
		{"an invited-but-not-accepted member is invisible", env.ShelterA, invitedToA, false,
			"§4.1 requires an ACTIVE membership; an invitation the user never accepted must " +
				"not grant the inviting tenant read access on its own"},
		{"a revoked member is invisible", env.ShelterA, revokedFromA, false,
			"revoking a membership has to take the visibility away, not just the access it " +
				"otherwise grants"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := userVisible(t, env, tc.shelter, tc.user); got != tc.want {
				t.Errorf("visible = %v, want %v. %s", got, tc.want, tc.why)
			}
		})
	}
}

// THE property citext buys over a lower(email) unique index.
//
// Both enforce case-insensitive uniqueness. They differ on LOOKUP: with a
// lower(email) index, `WHERE email = $1` silently misses a differently-cased
// address unless every caller remembers to normalise, on the write path and the
// read path both. That is correctness resting on discipline.
//
// So the query below is written the way a caller would write it if they had
// never heard of the problem — no lower(), no normalisation — and it has to
// work. If this test ever needs a lower() to pass, D7 has been undone.
//
// It runs as app_tenant, through the membership policy, because that is where
// the lookup actually happens.
func TestUsers_ADifferentlyCasedLookupStillFindsTheRow(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	id := uuid.New()
	stored := "Person." + uuid.NewString() + "@Example.ORG"

	if err := seedShelters(ctx, env.OwnerPool, env.ShelterA); err != nil {
		t.Fatalf("seeding shelters: %v", err)
	}
	if err := seedUser(ctx, env.OwnerPool, id, stored); err != nil {
		t.Fatalf("seeding the user: %v", err)
	}
	if err := seedMembership(ctx, env.OwnerPool, id, env.ShelterA); err != nil {
		t.Fatalf("seeding the membership: %v", err)
	}

	for _, lookup := range []struct {
		name  string
		email string
	}{
		{"all lower", lowerASCII(stored)},
		{"all upper", upperASCII(stored)},
		{"as stored", stored},
	} {
		t.Run(lookup.name, func(t *testing.T) {
			var found uuid.UUID
			err := db.WithTenant(ctx, env.TenantPool, env.ShelterA,
				func(ctx context.Context, tx pgx.Tx) error {
					return tx.QueryRow(ctx,
						`SELECT id FROM users WHERE email = $1`, lookup.email).Scan(&found)
				})
			if err != nil {
				t.Fatalf("a lookup by %q found nothing. With citext this must match "+
					"regardless of casing on either side; with a lower(email) index it "+
					"would not, and every call site would owe a normalisation nobody "+
					"can enforce: %v", lookup.email, err)
			}
			if found != id {
				t.Errorf("found user %s, want %s", found, id)
			}
		})
	}
}

// The uniqueness half. It runs as the OWNER, which is not a violation of this
// task's "never as the owner" rule but its consequence: app_tenant holds no
// INSERT on `users` at all in this phase, so there is no tenant-side path to
// test. Phase 02's registration flow is what will make this assertion runnable
// as an application role.
func TestUsers_DifferingCaseIsTheSameUser(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	stored := "Duplicate." + uuid.NewString() + "@Example.ORG"
	if err := seedUser(ctx, env.OwnerPool, uuid.New(), stored); err != nil {
		t.Fatalf("seeding the first user: %v", err)
	}

	_, err := env.OwnerPool.Exec(ctx,
		`INSERT INTO users (id, email, full_name) VALUES ($1, $2, $3)`,
		uuid.New(), upperASCII(stored), "A Duplicate")
	if err == nil {
		t.Fatal("a second user was created with the same address in different case. " +
			"Two accounts for one mailbox is an account-takeover path: whoever holds the " +
			"mailbox can reset the password of a row they never registered")
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("the duplicate was refused, but not by the unique constraint (23505), "+
			"so uniqueness is still unproven: %v", err)
	}
}

// The spec's third visibility scenario — an applicant is visible to the shelter
// they applied to — cannot be tested here: it needs `adoption_applications`,
// which is migration 00009 (T-01-027). Design D6 says that migration adds a
// SECOND permissive policy on `users` for the applicant path.
//
// This is the guard that stops the scenario from being quietly dropped. It
// asserts the equivalence in both directions, so it passes today with neither
// half present and goes red the day 00009 lands without the policy. Same shape
// as the deferred media foreign key in T-01-013.
func TestUsers_GetTheApplicantPolicyWhenAdoptionApplicationsLands(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	var applicationsExist bool
	if err := env.OwnerPool.QueryRow(ctx,
		`SELECT to_regclass('public.adoption_applications') IS NOT NULL`).
		Scan(&applicationsExist); err != nil {
		t.Fatalf("looking for adoption_applications: %v", err)
	}

	var policies int
	if err := env.OwnerPool.QueryRow(ctx, `
		SELECT count(*)
		FROM pg_policy p
		JOIN pg_class c ON c.oid = p.polrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relname = 'users'`).Scan(&policies); err != nil {
		t.Fatalf("counting the policies on users: %v", err)
	}

	switch {
	case applicationsExist && policies < 2:
		t.Errorf("`adoption_applications` exists but `users` still carries %d policy. The "+
			"spec scenario \"an applicant is visible to the shelter they applied to\" was "+
			"deferred out of T-01-014 only because the table did not exist yet; D6 says "+
			"migration 00009 adds the second permissive policy", policies)
	case !applicationsExist && policies != 1:
		t.Errorf("`users` carries %d policies but `adoption_applications` does not exist, "+
			"so one of them scopes visibility through something this phase never reviewed",
			policies)
	}
}

// lowerASCII and upperASCII fold only ASCII letters.
//
// strings.ToLower would work, but it also folds Unicode, and a test for
// case-insensitive email should not quietly depend on citext and Go agreeing
// about a Turkish dotless i. The addresses here are ASCII; the folding should be
// too, so a failure means what it says.
func lowerASCII(s string) string {
	out := []byte(s)
	for i, c := range out {
		if c >= 'A' && c <= 'Z' {
			out[i] = c - 'A' + 'a'
		}
	}

	return string(out)
}

func upperASCII(s string) string {
	out := []byte(s)
	for i, c := range out {
		if c >= 'a' && c <= 'z' {
			out[i] = c - 'a' + 'A'
		}
	}

	return string(out)
}
