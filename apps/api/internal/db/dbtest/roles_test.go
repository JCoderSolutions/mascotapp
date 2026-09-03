package dbtest_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
)

// CheckRoleCannotBypassRLS is pure, so every combination is asserted here
// rather than being inferred from one lucky environment.
func TestCheckRoleCannotBypassRLS(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		caps    dbtest.RoleCapabilities
		owner   string
		wantErr error
	}{
		{
			name:  "an ordinary application role passes",
			caps:  dbtest.RoleCapabilities{Name: "app_tenant"},
			owner: "mascotapp",
		},
		{
			// RLS does not apply to a superuser at all. The policies would be
			// present, correct, and completely inert.
			name:    "a superuser is refused",
			caps:    dbtest.RoleCapabilities{Name: "app_tenant", Super: true},
			owner:   "mascotapp",
			wantErr: dbtest.ErrRoleCanBypassRLS,
		},
		{
			// The Neon trap. neon_superuser carries BYPASSRLS and is granted
			// automatically to every role created through the Console, CLI or
			// API — so this is what a hand-provisioned app_tenant looks like.
			name:    "BYPASSRLS is refused",
			caps:    dbtest.RoleCapabilities{Name: "app_tenant", BypassRLS: true},
			owner:   "mascotapp",
			wantErr: dbtest.ErrRoleCanBypassRLS,
		},
		{
			name:    "both at once is still refused",
			caps:    dbtest.RoleCapabilities{Name: "app_tenant", Super: true, BypassRLS: true},
			owner:   "mascotapp",
			wantErr: dbtest.ErrRoleCanBypassRLS,
		},
		{
			// Connecting as the owner is the classic mistake. The owner does
			// not need BYPASSRLS to defeat a policy: without FORCE it bypasses
			// by default, and it can drop the policy outright.
			name:    "the owner role is refused even with clean attributes",
			caps:    dbtest.RoleCapabilities{Name: "mascotapp"},
			owner:   "mascotapp",
			wantErr: dbtest.ErrRoleIsOwner,
		},
		{
			name:    "an unnamed role is refused rather than assumed safe",
			caps:    dbtest.RoleCapabilities{},
			owner:   "mascotapp",
			wantErr: dbtest.ErrRoleUnknown,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := dbtest.CheckRoleCannotBypassRLS(tc.caps, tc.owner)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("rejected a usable role: %v", err)
				}

				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// The harness must hand out pools for both application roles, connected AS
// those roles. A suite that runs its isolation assertions as the owner proves
// nothing at all.
func TestPostgres_ProvidesPoolsForBothApplicationRoles(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	for want, pool := range map[string]dbtest.Querier{
		"app_tenant": env.TenantPool,
		"app_public": env.PublicPool,
		"app_auth":   env.AuthPool,
	} {
		t.Run(want, func(t *testing.T) {
			if pool == nil {
				t.Fatalf("the harness provides no pool for %s", want)
			}

			var current string
			if err := pool.QueryRow(ctx, `SELECT current_user`).Scan(&current); err != nil {
				t.Fatalf("reading current_user: %v", err)
			}
			if current != want {
				t.Fatalf("connected as %q, want %q", current, want)
			}
		})
	}
}

// This is the assertion that would catch a hand-provisioned Neon role before it
// reached production, and it is proven against a role that really can bypass:
// the container's owner is a superuser, so the guard must refuse it.
func TestGuard_RefusesARoleThatReallyCanBypassRLS(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	ownerCaps, err := dbtest.ReadRoleCapabilities(ctx, env.OwnerPool)
	if err != nil {
		t.Fatalf("reading the owner's capabilities: %v", err)
	}
	if !ownerCaps.Super && !ownerCaps.BypassRLS {
		t.Fatalf("the container owner %q can no longer bypass RLS, so this test proves "+
			"nothing; pick a different subject", ownerCaps.Name)
	}

	if err := dbtest.CheckRoleCannotBypassRLS(ownerCaps, ownerCaps.Name); err == nil {
		t.Fatal("the guard accepted a role that can bypass row-level security")
	}
}

func TestGuard_AcceptsTheApplicationRoles(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	for label, pool := range map[string]dbtest.Querier{
		"app_tenant": env.TenantPool,
		"app_public": env.PublicPool,
		"app_auth":   env.AuthPool,
	} {
		t.Run(label, func(t *testing.T) {
			caps, err := dbtest.ReadRoleCapabilities(ctx, pool)
			if err != nil {
				t.Fatalf("reading capabilities: %v", err)
			}
			if caps.Super {
				t.Errorf("%s is a superuser", label)
			}
			if caps.BypassRLS {
				t.Errorf("%s has BYPASSRLS: every policy in this schema would be inert", label)
			}
			if err := dbtest.CheckRoleCannotBypassRLS(caps, env.OwnerRole); err != nil {
				t.Errorf("the guard refused %s: %v", label, err)
			}
		})
	}
}

// The DoD of T-01-008 is that the guard runs in HARNESS SETUP, not in a test a
// `-run` filter can skip. Mutation testing found that nothing enforced it:
// deleting the guard call from the harness broke no test, because the only test
// exercising the guard called it directly.
//
// This asserts the harness itself did the work.
func TestPostgres_GuardsEveryApplicationPoolDuringSetup(t *testing.T) {
	env := dbtest.Postgres(t)

	guarded := env.GuardedRoles()
	for _, role := range []string{"app_tenant", "app_public", "app_auth"} {
		caps, ok := guarded[role]
		if !ok {
			t.Errorf("the harness handed out a %s pool without guarding it. Every A/B "+
				"assertion in this phase would then pass vacuously if that role could "+
				"bypass RLS", role)

			continue
		}
		if caps.Name != role {
			t.Errorf("guarded %q but recorded %q", role, caps.Name)
		}
		if caps.Super || caps.BypassRLS {
			t.Errorf("%s was recorded as guarded while still able to bypass RLS", role)
		}
	}
}
