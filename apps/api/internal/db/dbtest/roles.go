package dbtest

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Querier is the slice of a pgx pool or connection this guard needs.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// RoleCapabilities is what the guard reads about the role a connection is
// actually using. Not the role we configured, and not the role we intended —
// the one the server reports.
type RoleCapabilities struct {
	Name      string
	Super     bool
	BypassRLS bool
}

var (
	// ErrRoleCanBypassRLS means the connecting role can ignore every policy in
	// the schema.
	ErrRoleCanBypassRLS = errors.New("dbtest: the connecting role can bypass row-level security")

	// ErrRoleIsOwner means the suite is connected as the object owner.
	ErrRoleIsOwner = errors.New("dbtest: the connecting role is the object owner")

	// ErrRoleUnknown means the server reported no role name, so nothing can be
	// concluded. Refused rather than assumed safe.
	ErrRoleUnknown = errors.New("dbtest: could not determine the connecting role")
)

// readCapabilitiesSQL asks about `current_user`, deliberately, rather than
// about a role name we pass in. The question that matters is not "is app_tenant
// configured correctly" but "can the role this connection is actually using
// bypass the policies these tests are about to assert".
const readCapabilitiesSQL = `
SELECT current_user, r.rolsuper, r.rolbypassrls
FROM pg_roles r
WHERE r.rolname = current_user`

// ReadRoleCapabilities reports what the connection behind q is allowed to do.
func ReadRoleCapabilities(ctx context.Context, q Querier) (RoleCapabilities, error) {
	var caps RoleCapabilities
	if err := q.QueryRow(ctx, readCapabilitiesSQL).
		Scan(&caps.Name, &caps.Super, &caps.BypassRLS); err != nil {
		return RoleCapabilities{}, fmt.Errorf("dbtest: reading role capabilities: %w", err)
	}

	return caps, nil
}

// CheckRoleCannotBypassRLS is the guard from ADR-0002, and the only check that
// catches a hand-provisioned Neon role before it reaches production.
//
// The failure it exists for is the quietest one available in this project.
// Neon's `neon_superuser` carries BYPASSRLS and is granted automatically to
// every role created through the Console, CLI or API. Creating `app_tenant`
// from the dashboard would therefore make every policy in this schema a no-op
// in production, while this suite kept passing locally — the compose image has
// no `neon_superuser` for a role to inherit it from. Silent,
// environment-specific, and invisible to the tests written to catch exactly it.
//
// It is deliberately pure: the decision is asserted exhaustively in unit tests
// rather than inferred from whichever environment happens to run the suite.
func CheckRoleCannotBypassRLS(caps RoleCapabilities, ownerRole string) error {
	if caps.Name == "" {
		return ErrRoleUnknown
	}

	if caps.Super {
		return fmt.Errorf("%w: %q is a SUPERUSER, and row-level security does not apply "+
			"to superusers at all", ErrRoleCanBypassRLS, caps.Name)
	}
	if caps.BypassRLS {
		return fmt.Errorf("%w: %q carries BYPASSRLS. On Neon this is what a role created "+
			"through the Console, CLI or API looks like, and it makes every policy in this "+
			"schema a no-op", ErrRoleCanBypassRLS, caps.Name)
	}

	// The owner needs neither attribute to defeat a policy: without FORCE it
	// bypasses by default, and it can drop the policy outright. A suite that
	// asserts isolation as the owner proves nothing.
	if ownerRole != "" && caps.Name == ownerRole {
		return fmt.Errorf("%w: %q owns these objects, so it is not bound by their policies",
			ErrRoleIsOwner, caps.Name)
	}

	return nil
}

// guardRole refuses a pool whose role is not safe to assert isolation with, and
// returns what it verified.
//
// It runs during harness setup so every table test inherits it, rather than
// sitting in a standalone test that a `-run` filter can quietly skip. The
// capabilities come back so the harness can record that the check happened:
// mutation testing on 2026-08-29 showed that deleting this call broke nothing,
// because the only test exercising the guard called it directly. A guarantee
// nothing observes is not a guarantee.
func guardRole(
	ctx context.Context, label string, q Querier, ownerRole string,
) (RoleCapabilities, error) {
	caps, err := ReadRoleCapabilities(ctx, q)
	if err != nil {
		return RoleCapabilities{}, fmt.Errorf("guarding the %s pool: %w", label, err)
	}
	if err := CheckRoleCannotBypassRLS(caps, ownerRole); err != nil {
		return RoleCapabilities{}, fmt.Errorf(
			"the %s pool is unusable for isolation tests: %w\n\n"+
				"Every A/B assertion in this phase would pass vacuously against this role. "+
				"Roles must be created by the goose migration, in SQL, and never through a "+
				"provider console", label, err)
	}

	return caps, nil
}
