package rlstest_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gentleman/mascotapp/apps/api/internal/db"
	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
)

// B1, asserted: `app_tenant` cannot write the columns that decide its own
// privileges (`00015_column_grants`, decisions P2-D4 and P2-D5).
//
// Phase 01 granted `SELECT, INSERT, UPDATE, DELETE` on `shelters` and
// `memberships` table-wide, which means a tenant could set its own
// `status = 'verified'`, raise its own storage quota, and promote itself to
// `owner`. RLS never touched any of that: a policy decides WHICH ROWS a role
// reaches, and every one of these writes lands on a row the tenant legitimately
// owns. Only a privilege can decide which COLUMNS.
//
// Why an ISOLATED container. Before `00015` exists these statements SUCCEED —
// that is the point of the RED phase — and two of them are `DELETE FROM
// shelters` and `DELETE FROM memberships`. Run against the shared container they
// would delete the tenant rows every other test in this package builds on, and
// the resulting failures would look like anything except what they are. The
// couple of seconds this costs buys a RED phase that is safe to run.
func TestTenantGrants_CannotWriteThePrivilegeSensitiveColumns(t *testing.T) {
	env := dbtest.PostgresIsolated(t)
	ctx := context.Background()

	member := uuid.New()
	seedTenantFixture(t, env, member)

	// Each case is one column, and each column is one security property from
	// P2-D4/P2-D5. They are separate cases rather than one statement because a
	// single UPDATE naming all of them would be refused by the first column
	// PostgreSQL checked, leaving the rest unproven.
	// `scope` is the tenant the case runs as, and it is not always shelter A.
	//
	// The `shelters` DELETE case needs a shelter with NO memberships pointing at
	// it. Found in the RED phase, and it is worth writing down because it is a
	// trap: run against shelter A, that DELETE comes back 23503 — the foreign key
	// from `memberships` fires BEFORE the privilege is ever consulted. The case
	// would then be red today and green after `00015` while never once testing
	// the grant, which is a passing test that measures a foreign key.
	for _, tc := range []struct {
		name, sql, why string
		scope          uuid.UUID
		args           []any
	}{
		{
			name: "shelters_status",
			sql:  `UPDATE shelters SET status = 'verified' WHERE id = $1`,
			args: []any{env.ShelterA},
			why: "a tenant that can write its own status verifies itself, which is the " +
				"whole of LT-2. Registration still works because the DEFAULT supplies it",
		},
		{
			name: "shelters_verified_at",
			sql:  `UPDATE shelters SET verified_at = now() WHERE id = $1`,
			args: []any{env.ShelterA},
			why:  "same fact as status, second column of it",
		},
		{
			name: "shelters_verified_by",
			sql:  `UPDATE shelters SET verified_by = $2 WHERE id = $1`,
			args: []any{env.ShelterA, member},
			why:  "same fact as status, third column of it",
		},
		{
			name: "shelters_storage_quota_bytes",
			sql:  `UPDATE shelters SET storage_quota_bytes = 999999999999 WHERE id = $1`,
			args: []any{env.ShelterA},
			why:  "a self-raised quota is how a free tier dies",
		},
		{
			name: "shelters_storage_bytes_used",
			sql:  `UPDATE shelters SET storage_bytes_used = 0 WHERE id = $1`,
			args: []any{env.ShelterA},
			why: "the COUNTER the quota is checked against. Not in the proposal's table " +
				"and it is the same hole: writing it defeats Phase 04's quota check as " +
				"thoroughly as writing the quota",
		},
		{
			name: "shelters_slug",
			sql:  `UPDATE shelters SET slug = $2 WHERE id = $1`,
			args: []any{env.ShelterA, "stolen-slug"},
			why: "a slug change breaks every public URL already published. It is set once " +
				"at registration, which is why INSERT still carries it and UPDATE does not",
		},
		{
			name: "memberships_role",
			sql:  `UPDATE memberships SET role = 'owner' WHERE user_id = $1 AND shelter_id = $2`,
			args: []any{member, env.ShelterA},
			why: "ADR-0009's B1 finding verbatim: app_tenant could set role = 'owner' on " +
				"its own row. A promotion now requires a migration",
		},
		{
			name: "shelters_delete",
			// Shelter B, seeded with no membership, for the reason above.
			scope: env.ShelterB,
			sql:   `DELETE FROM shelters WHERE id = $1`,
			args:  []any{env.ShelterB},
			why: "deleting a shelter is destructive with no endpoint behind it. Archival " +
				"is a status change, and status is not writable either",
		},
		{
			name: "memberships_delete",
			sql:  `DELETE FROM memberships WHERE user_id = $1 AND shelter_id = $2`,
			args: []any{member, env.ShelterA},
			why: "revocation is status = 'revoked', which keeps the trail. A DELETE erases " +
				"the evidence that the membership ever existed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scope := tc.scope
			if scope == uuid.Nil {
				scope = env.ShelterA
			}

			err := db.WithTenant(ctx, env.TenantPool, scope,
				func(ctx context.Context, tx pgx.Tx) error {
					_, err := tx.Exec(ctx, tc.sql, tc.args...)

					return err
				})

			requireInsufficientPrivilege(t, err, "app_tenant wrote a column it must not: "+tc.why)
		})
	}
}

// Anti-vacuity, and it is not optional here.
//
// Every case above passes just as well against a role holding NO privilege on
// these tables at all — which would be a different schema and a broken product,
// since a shelter that cannot edit its own display name is not usable. What
// P2-D4/P2-D5 claim is that the grant NARROWS. That claim needs a permitted
// column to still write.
//
// It also separates the two refusal mechanisms. A missing privilege raises
// 42501; a policy that excludes the row affects ZERO rows and raises nothing. By
// asserting one row actually changed, these cases prove the tenant policy still
// admits this row — so the 42501s above came from the grant and from nothing
// else.
func TestTenantGrants_StillWriteThePermittedColumns(t *testing.T) {
	env := dbtest.PostgresIsolated(t)
	ctx := context.Background()

	member := uuid.New()
	seedTenantFixture(t, env, member)

	for _, tc := range []struct {
		name, sql, read string
		args            []any
		want            string
	}{
		{
			name: "shelters_legal_name",
			sql:  `UPDATE shelters SET legal_name = $2 WHERE id = $1`,
			args: []any{env.ShelterA, "Refugio Legal S.C."},
			read: `SELECT legal_name FROM shelters WHERE id = $1`,
			want: "Refugio Legal S.C.",
		},
		{
			name: "memberships_status",
			sql:  `UPDATE memberships SET status = 'revoked' WHERE user_id = $2 AND shelter_id = $1`,
			args: []any{env.ShelterA, member},
			read: `SELECT status FROM memberships WHERE shelter_id = $1`,
			want: "revoked",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			err := db.WithTenant(ctx, env.TenantPool, env.ShelterA,
				func(ctx context.Context, tx pgx.Tx) error {
					tag, err := tx.Exec(ctx, tc.sql, tc.args...)
					if err != nil {
						return err
					}
					if tag.RowsAffected() != 1 {
						t.Errorf("the UPDATE touched %d rows, want 1. The statement was "+
							"permitted but changed nothing, so this proves the grant is "+
							"wide enough and nothing about whether it works",
							tag.RowsAffected())
					}

					return tx.QueryRow(ctx, tc.read, env.ShelterA).Scan(&got)
				})
			if err != nil {
				t.Fatalf("writing a PERMITTED column: %v. The grant did not narrow, it "+
					"broke -- a shelter that cannot edit its own fields is not a product", err)
			}
			if got != tc.want {
				t.Errorf("read back %q, want %q: the UPDATE reported success without writing",
					got, tc.want)
			}
		})
	}
}

// A characterization test for a hole `00015` does NOT close, pinned so that the
// debt is something somebody finds rather than something somebody rediscovers.
//
// `INSERT` on memberships still carries `role` (P2-D5), so a member of shelter A
// can insert an `owner` membership for an accomplice account INSIDE shelter A.
// A column grant cannot close it: closing it needs the database to know WHO is
// acting, and under `app_tenant` there is deliberately no user GUC (P2-D1).
//
// Three things bound it today: `UNIQUE (user_id, shelter_id)` blocks a second
// row for oneself, the tenant policy confines it to one shelter, and domain RBAC
// refuses `role IN ('owner','admin')` to anyone who is not already `owner`.
//
// Revoking INSERT outright was considered and rejected: registration inserts the
// founder's owner membership, and moving that to `app_auth` would need a policy
// of `user_id = app.user_id`, under which ANY user could join ANY existing
// shelter. That is a full tenant takeover — strictly worse than the accomplice
// path it would close.
//
// TO WHOEVER READS THIS IN PHASE 03: the invitation endpoint is where this rule
// becomes expressible. When it lands and narrows the path, DELETE THIS TEST. It
// exists to describe today's behaviour, not to defend it.
func TestMembershipInsert_CanStillMintAnOwner(t *testing.T) {
	env := dbtest.PostgresIsolated(t)
	ctx := context.Background()

	member := uuid.New()
	seedTenantFixture(t, env, member)

	accomplice := uuid.New()
	if err := seedUser(ctx, env.OwnerPool, accomplice, "accomplice-"+uuid.NewString()+"@example.test"); err != nil {
		t.Fatalf("seeding the accomplice user: %v", err)
	}

	err := db.WithTenant(ctx, env.TenantPool, env.ShelterA,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO memberships (id, user_id, shelter_id, role, status)
				 VALUES ($1, $2, $3, 'owner', 'active')`,
				uuid.New(), accomplice, env.ShelterA)

			return err
		})
	if err != nil {
		t.Fatalf("minting an owner membership for an accomplice failed with %v.\n\n"+
			"If a Phase 03 change closed this path, that is GOOD NEWS and this test has "+
			"done its job: delete it, and delete the residual paragraph in P2-D5 that "+
			"names it. Do not widen the grant to make it pass again", err)
	}

	// The bound that makes the residual survivable: it reaches exactly one
	// shelter, the one the tenant is already scoped to.
	var reachable int
	if err := env.OwnerPool.QueryRow(ctx,
		`SELECT count(DISTINCT shelter_id) FROM memberships WHERE user_id = $1`,
		accomplice).Scan(&reachable); err != nil {
		t.Fatalf("counting the accomplice's shelters: %v", err)
	}
	if reachable != 1 {
		t.Errorf("the accomplice landed in %d shelters, want 1. The tenant policy is what "+
			"confines this hole to a single shelter; if it reaches more, the residual is "+
			"no longer bounded and P2-D5's argument for tolerating it does not hold",
			reachable)
	}
}

// seedTenantFixture creates the shelter, the user and the membership the cases
// above act on, as the owner. Fixture setup is the one thing the owner is for.
func seedTenantFixture(t *testing.T, env *dbtest.Env, member uuid.UUID) {
	t.Helper()

	ctx := context.Background()
	// Shelter B carries no membership on purpose: it is the row the `shelters`
	// DELETE case acts on, and a membership would make a foreign key answer that
	// case instead of the privilege system.
	if err := seedShelters(ctx, env.OwnerPool, env.ShelterA, env.ShelterB); err != nil {
		t.Fatalf("seeding the shelters: %v", err)
	}
	if err := seedUser(ctx, env.OwnerPool, member, "member-"+uuid.NewString()+"@example.test"); err != nil {
		t.Fatalf("seeding the member user: %v", err)
	}
	if err := seedMembership(ctx, env.OwnerPool, member, env.ShelterA); err != nil {
		t.Fatalf("seeding the membership: %v", err)
	}
}
