package rlstest_test

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
)

// P2-D1's whole argument: app_auth is a THIRD door, not a second GUC on
// app_tenant's existing one. app_tenant reads app.shelter_id and never
// app.user_id; app_auth reads app.user_id and never app.shelter_id. The two
// GUCs are never concurrent -- different pools, different roles, different
// transactions -- so no policy is ever evaluated beside a variable it does
// not read, PROVIDED no policy ever names the other role's GUC. This file is
// what keeps that true after migration 00013 stops being the newest one.

// policyRow is one row of the pg_policy scan below: the table, the policy,
// the role it applies to, and the expressions PostgreSQL renders for USING
// and WITH CHECK -- either may be nil, since not every policy carries both.
type policyRow struct {
	table     string
	policy    string
	role      string
	using     *string
	withCheck *string
}

// mentions reports whether either expression on this policy row names a GUC.
//
// It reads the rendered expression rather than parsing it -- the same choice
// policyMentions in public_catalog_test.go makes, for the same reason: this
// is deciding whether a name appears in an expression, not validating the
// expression's grammar.
func (p policyRow) mentions(name string) bool {
	for _, expr := range []*string{p.using, p.withCheck} {
		if expr != nil && strings.Contains(*expr, name) {
			return true
		}
	}

	return false
}

// readPolicyGUCs reads every policy in schema public that applies to
// app_tenant or app_auth. polroles is an oid array, so the join goes through
// pg_roles rather than comparing names directly.
func readPolicyGUCs(t *testing.T, env *dbtest.Env) []policyRow {
	t.Helper()

	rows, err := env.OwnerPool.Query(context.Background(), `
		SELECT c.relname, p.polname, r.rolname,
		       pg_get_expr(p.polqual, p.polrelid),
		       pg_get_expr(p.polwithcheck, p.polrelid)
		FROM pg_policy p
		JOIN pg_class c ON c.oid = p.polrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_roles r ON r.oid = ANY(p.polroles)
		WHERE n.nspname = 'public' AND r.rolname IN ('app_tenant', 'app_auth')
		ORDER BY c.relname, p.polname, r.rolname`)
	if err != nil {
		t.Fatalf("reading the policy catalog: %v", err)
	}
	defer rows.Close()

	var out []policyRow
	for rows.Next() {
		var row policyRow
		if err := rows.Scan(
			&row.table, &row.policy, &row.role, &row.using, &row.withCheck,
		); err != nil {
			t.Fatalf("scanning a policy row: %v", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the policy catalog: %v", err)
	}

	return out
}

// The scan against the real, currently-migrated schema. A future policy that
// widens app_tenant to also check app.user_id, or app_auth to also check
// app.shelter_id, fails here -- before it fails anywhere a human reviewing a
// migration diff would notice the two doors had quietly become one.
func TestPolicies_DoNotCrossGUCs(t *testing.T) {
	env := dbtest.Postgres(t)

	policies := readPolicyGUCs(t, env)
	if len(policies) == 0 {
		t.Fatal("the scan found no app_tenant or app_auth policies at all, so this test " +
			"would pass vacuously")
	}

	var sawTenant, sawAuth bool
	var crossed []string
	for _, p := range policies {
		switch p.role {
		case "app_tenant":
			sawTenant = true
			if p.mentions("app.user_id") {
				crossed = append(crossed, fmt.Sprintf(
					"%s.%s (app_tenant) reads app.user_id -- app_tenant reads app.shelter_id "+
						"only (P2-D1)", p.table, p.policy))
			}
		case "app_auth":
			sawAuth = true
			if p.mentions("app.shelter_id") {
				crossed = append(crossed, fmt.Sprintf(
					"%s.%s (app_auth) reads app.shelter_id -- app_auth reads app.user_id "+
						"only (P2-D1)", p.table, p.policy))
			}
		}
	}

	// Anti-vacuity, for both halves separately: a scan that silently found
	// zero app_auth policies would pass this test while proving nothing about
	// the half it exists for.
	if !sawTenant {
		t.Error("the scan found no app_tenant policy at all, so the app_tenant half of this " +
			"assertion never ran")
	}
	if !sawAuth {
		t.Error("the scan found no app_auth policy at all, so the app_auth half of this " +
			"assertion never ran")
	}

	if len(crossed) > 0 {
		sort.Strings(crossed)
		t.Errorf("a policy reads the other door's GUC:\n  %s", strings.Join(crossed, "\n  "))
	}
}

// The pure half: mentions is what the assertion above rests its verdict on,
// so it gets its own table-driven proof independent of a real database.
func TestPolicyRow_Mentions(t *testing.T) {
	t.Parallel()

	tenantScoped := "shelter_id = current_setting('app.shelter_id')::uuid"
	authScoped := "user_id = current_setting('app.user_id')::uuid"

	cases := []struct {
		name string
		row  policyRow
		guc  string
		want bool
	}{
		{
			name: "present in USING",
			row:  policyRow{using: &authScoped},
			guc:  "app.user_id",
			want: true,
		},
		{
			name: "present in WITH CHECK only",
			row:  policyRow{withCheck: &authScoped},
			guc:  "app.user_id",
			want: true,
		},
		{
			name: "the other GUC entirely, in both",
			row:  policyRow{using: &tenantScoped, withCheck: &tenantScoped},
			guc:  "app.user_id",
			want: false,
		},
		{
			name: "neither expression set",
			row:  policyRow{},
			guc:  "app.user_id",
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := tc.row.mentions(tc.guc); got != tc.want {
				t.Errorf("mentions(%q) = %v, want %v", tc.guc, got, tc.want)
			}
		})
	}
}
