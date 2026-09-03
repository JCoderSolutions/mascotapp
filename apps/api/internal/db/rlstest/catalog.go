// Package rlstest holds the schema-wide row-level-security assertions: the
// declaration of which tables exist and what protects each one, and the catalog
// meta-test that makes forgetting a table a build failure rather than a silent
// gap.
//
// It is a non-test package so the declaration below can be a single source of
// truth shared by the meta-test, the A/B suite and the child-orphan suite. It is
// imported only from tests and never links into a binary.
package rlstest

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Schema is the canonical classification of every relation this phase's
// migrations create. It mirrors the "Tenant table set" table in the
// tenant-isolation spec, and the counts are asserted against that table.
//
// Adding a migration means adding the table here and deleting its Pending line.
// Both halves are enforced: an unclassified table fails ClassifyAll, and a
// table that landed while still listed as pending fails CheckPending.
var Schema = Classification{
	// A table is a tenant table when each of its rows belongs to exactly one
	// shelter. `shelters` is in the set scoped by its own `id` rather than by a
	// `shelter_id` column.
	Tenant: []string{
		"adoption_applications",
		"application_events",
		"application_notes",
		"audit_log",
		"documents",
		"form_submissions",
		"form_template_versions",
		"form_templates",
		"media",
		"memberships",
		"pet_health_records",
		"pet_media",
		"pet_status_history",
		"pets",
		"shelters",
	},

	// Tenant tables whose `shelter_id` is denormalised from a parent. Each one
	// needs D5's composite foreign key to `parent (id, shelter_id)`: referential
	// integrity checks always bypass row security, so a single-column key would
	// let a child row point at another shelter's parent.
	TenantChildren: []string{
		"adoption_applications",
		"application_events",
		"application_notes",
		"documents",
		"form_submissions",
		"form_template_versions",
		"pet_health_records",
		"pet_media",
		"pet_status_history",
	},

	// Model tables whose rows do not belong to exactly one shelter: `users` and
	// `refresh_tokens` span shelters, `species` and `breeds` are global
	// reference data. They still carry RLS — see NoPolicy and the doc on
	// CheckProtection.
	NonTenantModel: []string{
		"breeds",
		"refresh_tokens",
		"species",
		"users",
	},

	// Created by goose itself rather than by a migration, so no migration in
	// this repository controls its flags.
	Infrastructure: []string{
		"goose_db_version",
	},

	// Tables whose rows may be inserted and read but never updated, deleted or
	// truncated by any application role. §4.5 names them the trail; the
	// append-only-audit spec enumerates the four layers each one carries.
	//
	// Declared here rather than checked table by table so the enforcement suite
	// is driven by ENUMERATION: a third append-only table gets its assertions
	// the day it is added to this list, with no test to remember to write. Two
	// hand-written cases is how `form_submissions` slipped through T-01-025.
	AppendOnly: []string{
		"application_events",
		"audit_log",
	},

	// The single declared exception to the "at least one policy" rule.
	//
	// EMPTY as of T-02-004: `refresh_tokens` left this set the day migration
	// 00013 gave it a real policy (`auth_own_sessions`, P2-D2). The exemption
	// was always meant to self-expire the moment Phase 02 chose its access
	// path — see the comment TestQueries_ExerciseEveryDeclaredTable carries at
	// `query_test.go:95`, which derives its own exemption from this list and
	// therefore now demands a query for `refresh_tokens` where none existed.
	NoPolicy: []string{},

	// Declared tables whose migration has not landed yet, each with the task
	// that creates it. A protection assertion over a table that does not exist
	// is a no-op, and a suite full of silent no-ops reads as proof while
	// proving nothing — so the ledger is explicit and the meta-test reports how
	// much of the declaration it actually verified.
	Pending: map[string]string{
		// shelters, users, memberships and refresh_tokens were removed here by
		// T-01-013, when migration 00002 landed them; media by T-01-017;
		// species and breeds by T-01-018; pets by T-01-019; pet_media,
		// pet_health_records and pet_status_history by T-01-020; form_templates
		// and form_template_versions by T-01-023; form_submissions by T-01-025;
		// adoption_applications by T-01-027; application_events and
		// application_notes by T-01-029; documents by T-01-031; audit_log by
		// T-01-032. The ledger is now EMPTY: every declared table exists and
		// every protection assertion runs against a real relation.
	},

	// A table whose RLS predates the migration that gives it its own first
	// policy, keyed to the goose version number of that migration. This is
	// NOT the same gap Pending covers: Pending is for a table that does not
	// exist yet, and CheckProtection (unversioned, HEAD-only) never consults
	// this map at all. PolicyLandsAt exists for CheckProtectionAt, which the
	// stepwise rollback walk (migrate_roundtrip_test.go) uses to inspect the
	// schema at every INTERMEDIATE version between the migrations, not only
	// at HEAD.
	//
	// refresh_tokens is the one entry: RLS has been ON and FORCED on it since
	// 00002, but T-02-004 is what removes it from NoPolicy, because 00013 is
	// what gives it a real policy (auth_own_sessions, P2-D2). Versions 00002
	// through 00012 are real, applied states the stepwise walk passes
	// through, and at every one of them refresh_tokens legitimately carries
	// zero policies — not because a rollback broke something, but because
	// migration 00013 has not run yet. Without this entry the walk would read
	// that gap as a broken rollback instead of the ordinary, un-arrived-at
	// state it is.
	PolicyLandsAt: map[string]int64{
		"refresh_tokens": 13,
	},
}

// Classification declares every relation the schema is allowed to contain and
// what each one is.
type Classification struct {
	Tenant         []string
	TenantChildren []string
	AppendOnly     []string
	NonTenantModel []string
	Infrastructure []string
	NoPolicy       []string
	Pending        map[string]string
	PolicyLandsAt  map[string]int64
}

var (
	// ErrUnclassifiedRelation means a relation exists in schema public that no
	// set accounts for. It is the failure this whole file exists to produce.
	ErrUnclassifiedRelation = errors.New("rlstest: a relation in schema public is unclassified")

	// ErrDeclarationInvalid means the declaration contradicts itself and cannot
	// be trusted to check anything.
	ErrDeclarationInvalid = errors.New("rlstest: the classification is internally inconsistent")

	// ErrPendingTableExists means a table listed as not-yet-created has landed.
	ErrPendingTableExists = errors.New("rlstest: a pending table now exists")

	// ErrTableMissing means a declared table is absent and is not accounted for
	// by the pending ledger.
	ErrTableMissing = errors.New("rlstest: a declared table is missing")

	// ErrRLSNotEnabled means relrowsecurity is false, which makes every policy
	// on the table inert.
	ErrRLSNotEnabled = errors.New("rlstest: row level security is not enabled")

	// ErrRLSNotForced means relforcerowsecurity is false, so the table's owner
	// bypasses its policies.
	ErrRLSNotForced = errors.New("rlstest: row level security is not forced")

	// ErrNoPolicy means a model table carries no policy and is not a declared
	// default-deny exception.
	ErrNoPolicy = errors.New("rlstest: the table carries no policy")
)

// ModelTables is every table a migration creates: the tenant set plus the
// non-tenant set, sorted. The infrastructure exemption is not one.
func (c Classification) ModelTables() []string {
	all := make([]string, 0, len(c.Tenant)+len(c.NonTenantModel))
	all = append(all, c.Tenant...)
	all = append(all, c.NonTenantModel...)
	sort.Strings(all)

	return all
}

// IsModelTable reports whether the named relation is one a migration creates.
func (c Classification) IsModelTable(name string) bool {
	return contains(c.Tenant, name) || contains(c.NonTenantModel, name)
}

// Validate checks the declaration against itself.
//
// Every assertion in this package is only as good as the lists above, and the
// ways those lists go wrong are quiet: a table classified twice passes the
// exhaustiveness rule while being covered by two contradictory rules, and a
// duplicate inside one set inflates the count the spec pins.
func (c Classification) Validate() error {
	var problems []string

	seen := map[string]string{}
	for _, set := range []struct {
		label string
		names []string
	}{
		{"Tenant", c.Tenant},
		{"NonTenantModel", c.NonTenantModel},
		{"Infrastructure", c.Infrastructure},
	} {
		for _, name := range set.names {
			if where, ok := seen[name]; ok {
				if where == set.label {
					problems = append(problems, fmt.Sprintf(
						"%q is listed twice in %s, which inflates the declared count",
						name, set.label))
				} else {
					problems = append(problems, fmt.Sprintf(
						"%q is declared in both %s and %s", name, where, set.label))
				}

				continue
			}
			seen[name] = set.label
		}
	}

	for _, name := range c.TenantChildren {
		if !contains(c.Tenant, name) {
			problems = append(problems, fmt.Sprintf(
				"%q is a tenant child but not a tenant table, so it would never get an "+
					"A/B case", name))
		}
	}

	for _, name := range sortedKeys(c.Pending) {
		if !c.IsModelTable(name) {
			problems = append(problems, fmt.Sprintf(
				"%q is pending but is not a declared model table", name))
		}
	}

	for _, name := range c.NoPolicy {
		if !c.IsModelTable(name) {
			problems = append(problems, fmt.Sprintf(
				"%q is a declared policy exception but is not a declared model table", name))
		}
	}

	for _, name := range sortedInt64Keys(c.PolicyLandsAt) {
		if !c.IsModelTable(name) {
			problems = append(problems, fmt.Sprintf(
				"%q is in PolicyLandsAt but is not a declared model table", name))
		}
		if contains(c.NoPolicy, name) {
			problems = append(problems, fmt.Sprintf(
				"%q is declared both permanently policy-free (NoPolicy) and as gaining a "+
					"policy at a later version (PolicyLandsAt), which is a contradiction", name))
		}
	}

	for _, dup := range duplicates(c.TenantChildren) {
		problems = append(problems, fmt.Sprintf(
			"%q is listed twice in TenantChildren", dup))
	}

	// Same two checks the child set gets, for the same two reasons: an
	// append-only table outside the model set would never be reached by the
	// enforcement suite, and a duplicate inflates the count the spec pins.
	for _, name := range c.AppendOnly {
		if !c.IsModelTable(name) {
			problems = append(problems, fmt.Sprintf(
				"%q is declared append-only but is not a declared model table, so the "+
					"enforcement suite would never reach it", name))
		}
	}

	for _, dup := range duplicates(c.AppendOnly) {
		problems = append(problems, fmt.Sprintf(
			"%q is listed twice in AppendOnly", dup))
	}

	if len(problems) == 0 {
		return nil
	}

	return fmt.Errorf("%w: %s", ErrDeclarationInvalid, strings.Join(problems, "; "))
}

// ClassifyAll is the exhaustiveness rule: every relation that exists in schema
// public must be in exactly one declared set.
//
// It says nothing about declared tables that do not exist yet — migrations land
// one at a time, and CheckPending owns that direction.
//
// Every offender is reported at once. A rule that surfaces one missing table per
// run turns a five-table migration into five round trips.
func (c Classification) ClassifyAll(existing []string) error {
	var unknown []string
	for _, name := range existing {
		if !c.IsModelTable(name) && !contains(c.Infrastructure, name) {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)

	return fmt.Errorf("%w: %s", ErrUnclassifiedRelation, strings.Join(unknown, ", "))
}

// CheckPending keeps the ledger of not-yet-created tables honest in both
// directions: a pending table that has landed must be removed from the ledger
// (which switches its protection assertions on), and a declared table that is
// absent must be accounted for by the ledger.
func (c Classification) CheckPending(existing []string) error {
	present := map[string]bool{}
	for _, name := range existing {
		present[name] = true
	}

	var problems []error
	for _, name := range sortedKeys(c.Pending) {
		if present[name] {
			problems = append(problems, fmt.Errorf(
				"%w: %q landed in %s — delete its line from Pending so its protection is "+
					"actually asserted", ErrPendingTableExists, name, c.Pending[name]))
		}
	}
	for _, name := range c.ModelTables() {
		if _, pending := c.Pending[name]; !pending && !present[name] {
			problems = append(problems, fmt.Errorf(
				"%w: %q is declared and is not listed as pending, but does not exist",
				ErrTableMissing, name))
		}
	}

	return errors.Join(problems...)
}

// TableProtection is what the catalog reports about one relation.
type TableProtection struct {
	Name        string
	RLSEnabled  bool
	RLSForced   bool
	PolicyCount int
}

// CheckProtection asserts the row-level-security guarantees for one relation.
//
// The non-tenant tables are held to the same standard as the tenant ones, and
// that is not ceremony. A policy on a table whose RLS is not enabled is INERT:
// `users` is protected only by D6's EXISTS-through-memberships policy, so
// exempting it would expose every user row to every tenant while the policy
// still read as correct in the migration.
//
// FORCE matters for the same reason at a different layer. Migrations run as the
// object owner, and an owner without FORCE bypasses its own tables' policies —
// so a suite that ran as the owner would prove nothing, and a production role
// that happened to own a table would silently be exempt.
func (c Classification) CheckProtection(t TableProtection) error {
	return c.checkProtection(t, contains(c.NoPolicy, t.Name))
}

// CheckProtectionAt is CheckProtection with one further allowance: a table
// entered in PolicyLandsAt is not held to "must carry a policy" at any
// version STRICTLY BEFORE the one recorded there. It exists for the stepwise
// rollback walk (migrate_roundtrip_test.go), which inspects the schema at
// every intermediate version, not only at HEAD — CheckProtection has no
// notion of "at a version" and does not consult PolicyLandsAt at all, so the
// full-schema meta-test still demands a policy unconditionally.
func (c Classification) CheckProtectionAt(t TableProtection, at int64) error {
	exempt := contains(c.NoPolicy, t.Name)
	if landsAt, tracked := c.PolicyLandsAt[t.Name]; tracked && at < landsAt {
		exempt = true
	}

	return c.checkProtection(t, exempt)
}

// checkProtection is the shared rule CheckProtection and CheckProtectionAt
// both run. policyExempt is the one thing that differs between them: whether
// a zero policy count is acceptable for this table at this point in time.
func (c Classification) checkProtection(t TableProtection, policyExempt bool) error {
	if contains(c.Infrastructure, t.Name) {
		return nil
	}
	if !c.IsModelTable(t.Name) {
		return fmt.Errorf("%w: %q", ErrUnclassifiedRelation, t.Name)
	}

	var problems []error
	if !t.RLSEnabled {
		problems = append(problems, fmt.Errorf(
			"%w: %q — every policy on it is inert", ErrRLSNotEnabled, t.Name))
	}
	if !t.RLSForced {
		problems = append(problems, fmt.Errorf(
			"%w: %q — the owner bypasses its policies", ErrRLSNotForced, t.Name))
	}
	if t.PolicyCount == 0 && !policyExempt {
		problems = append(problems, fmt.Errorf(
			"%w: %q is not a declared default-deny table, so no policy means no access "+
				"rather than isolation", ErrNoPolicy, t.Name))
	}

	return errors.Join(problems...)
}

// readTablesSQL reads relations of kind `r` (ordinary) and `p` (partitioned).
// The spec names kind `r`; `p` is included so that partitioning a table later
// cannot make it vanish from this test, which is the silent gap the task
// forbids.
const readTablesSQL = `
SELECT c.relname,
       c.relrowsecurity,
       c.relforcerowsecurity,
       (SELECT count(*) FROM pg_policy p WHERE p.polrelid = c.oid)
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = 'public'
  AND c.relkind IN ('r', 'p')
ORDER BY c.relname`

// Queryer is the slice of a pgx pool or connection the catalog reader needs.
type Queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// ReadTables reports every relation in schema public along with its
// row-level-security flags and policy count.
//
// It is read as the object owner on purpose: this is introspection, not an
// isolation assertion, and a role that could not see the whole catalog would
// report a partial schema as a complete one.
func ReadTables(ctx context.Context, q Queryer) ([]TableProtection, error) {
	rows, err := q.Query(ctx, readTablesSQL)
	if err != nil {
		return nil, fmt.Errorf("rlstest: reading the catalog: %w", err)
	}
	defer rows.Close()

	var tables []TableProtection
	for rows.Next() {
		var t TableProtection
		if err := rows.Scan(&t.Name, &t.RLSEnabled, &t.RLSForced, &t.PolicyCount); err != nil {
			return nil, fmt.Errorf("rlstest: scanning a catalog row: %w", err)
		}
		tables = append(tables, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rlstest: reading the catalog: %w", err)
	}

	return tables, nil
}

func contains(set []string, name string) bool {
	for _, candidate := range set {
		if candidate == name {
			return true
		}
	}

	return false
}

func duplicates(set []string) []string {
	seen := map[string]bool{}
	var dups []string
	for _, name := range set {
		if seen[name] {
			dups = append(dups, name)
		}
		seen[name] = true
	}

	return dups
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys
}

func sortedInt64Keys(m map[string]int64) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys
}
