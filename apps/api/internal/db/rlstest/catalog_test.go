package rlstest_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gentleman/mascotapp/apps/api/internal/db/dbtest"
	"github.com/gentleman/mascotapp/apps/api/internal/db/rlstest"
)

// The declaration is the single source of truth for which tables exist and what
// protects each one. It has to be checked against ITSELF before it is checked
// against a database: a name in two sets, or a child that is not a tenant table,
// would silently weaken every assertion built on top of it.
func TestSchema_DeclarationIsInternallyConsistent(t *testing.T) {
	t.Parallel()

	if err := rlstest.Schema.Validate(); err != nil {
		t.Fatalf("the declared classification is inconsistent: %v", err)
	}
}

// The counts are spelled out in the spec's "Tenant table set" table, so a
// silent drift between the spec and this file is a test failure.
func TestSchema_MatchesTheDeclaredCounts(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		what string
		got  int
		want int
	}{
		{"tenant tables", len(rlstest.Schema.Tenant), 15},
		{"tenant child tables", len(rlstest.Schema.TenantChildren), 9},
		{"append-only tables", len(rlstest.Schema.AppendOnly), 2},
		{"non-tenant model tables", len(rlstest.Schema.NonTenantModel), 4},
		{"infrastructure exemptions", len(rlstest.Schema.Infrastructure), 1},
		{"model tables", len(rlstest.Schema.ModelTables()), 19},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: got %d, want %d", tc.what, tc.got, tc.want)
		}
	}
}

func TestValidate_RejectsAnInconsistentDeclaration(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		c    rlstest.Classification
		// wants is a fragment the failure must carry, so each case proves the
		// check it breaks is the one that fired. Asserting only that SOME error
		// came back lets one loud check cover for several silent ones.
		wants string
		why   string
	}{
		{
			name:  "a table in two sets",
			wants: "declared in both",
			c: rlstest.Classification{
				Tenant:         []string{"pets"},
				NonTenantModel: []string{"pets"},
				Infrastructure: []string{"goose_db_version"},
			},
			why: "a table classified twice satisfies the exhaustiveness check while being " +
				"covered by two contradictory rules",
		},
		{
			name:  "a child that is not a tenant table",
			wants: "is a tenant child but not a tenant table",
			c: rlstest.Classification{
				Tenant:         []string{"pets"},
				TenantChildren: []string{"pet_media"},
				Infrastructure: []string{"goose_db_version"},
			},
			why: "the child set drives the composite-FK assertions; a child outside the " +
				"tenant set would never get an A/B case",
		},
		{
			name:  "a pending table nobody declared",
			wants: "is pending but is not a declared model table",
			c: rlstest.Classification{
				Tenant:         []string{"pets"},
				Infrastructure: []string{"goose_db_version"},
				Pending:        map[string]string{"ghosts": "T-01-999"},
			},
			why: "a pending entry for an unclassified table would be removed on arrival " +
				"and then never checked by anything",
		},
		{
			name:  "a policy exception for a table nobody declared",
			wants: "is a declared policy exception",
			c: rlstest.Classification{
				Tenant:         []string{"pets"},
				Infrastructure: []string{"goose_db_version"},
				NoPolicy:       []string{"ghosts"},
			},
			why: "an exception that matches no table is dead weight that reads as a " +
				"deliberate, reviewed carve-out",
		},
		{
			name:  "an append-only table nobody declared",
			wants: "is declared append-only but is not a declared model table",
			c: rlstest.Classification{
				Tenant:         []string{"pets"},
				Infrastructure: []string{"goose_db_version"},
				AppendOnly:     []string{"ghosts"},
			},
			why: "the enforcement suite is driven by this list, so a name outside the " +
				"model set would be a case that never runs and reads as one that passed",
		},
		{
			name:  "a duplicate in the append-only set",
			wants: "listed twice in AppendOnly",
			c: rlstest.Classification{
				Tenant:         []string{"audit_log"},
				AppendOnly:     []string{"audit_log", "audit_log"},
				Infrastructure: []string{"goose_db_version"},
			},
			why: "a duplicate inflates the count the spec pins, exactly as it would in " +
				"TenantChildren",
		},
		{
			name:  "a duplicate in the child set",
			wants: "listed twice in TenantChildren",
			c: rlstest.Classification{
				Tenant:         []string{"pet_media"},
				TenantChildren: []string{"pet_media", "pet_media"},
				Infrastructure: []string{"goose_db_version"},
			},
			why: "the child set is not covered by the cross-set check, so a duplicate " +
				"there would inflate the count the spec pins with nothing to catch it",
		},
		{
			name:  "a duplicate inside one set",
			wants: "listed twice in Tenant",
			c: rlstest.Classification{
				Tenant:         []string{"pets", "pets"},
				Infrastructure: []string{"goose_db_version"},
			},
			why: "a duplicate inflates the declared count, which is what the count " +
				"assertion exists to protect",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.c.Validate()
			if err == nil {
				t.Fatalf("Validate accepted a declaration where %s", tc.why)
			}
			if !strings.Contains(err.Error(), tc.wants) {
				t.Fatalf("Validate failed, but not on the rule this case breaks.\n"+
					"  want a failure mentioning: %q\n  got: %v", tc.wants, err)
			}
		})
	}
}

// The exhaustiveness rule. This is the assertion that turns "somebody forgot to
// classify the new table" from a silent gap into a build failure.
func TestClassifyAll(t *testing.T) {
	t.Parallel()

	c := rlstest.Classification{
		Tenant:         []string{"pets"},
		NonTenantModel: []string{"users"},
		Infrastructure: []string{"goose_db_version"},
	}

	t.Run("accepts relations that are all classified", func(t *testing.T) {
		t.Parallel()

		if err := c.ClassifyAll([]string{"goose_db_version", "pets", "users"}); err != nil {
			t.Fatalf("rejected a fully classified schema: %v", err)
		}
	})

	t.Run("accepts a schema where only some declared tables exist yet", func(t *testing.T) {
		t.Parallel()

		// Migrations land one at a time. The rule is about what EXISTS being
		// classified, not about the declaration being complete.
		if err := c.ClassifyAll([]string{"goose_db_version"}); err != nil {
			t.Fatalf("rejected a partially migrated schema: %v", err)
		}
	})

	t.Run("names the unclassified relation", func(t *testing.T) {
		t.Parallel()

		err := c.ClassifyAll([]string{"goose_db_version", "pets", "adoption_applications"})
		if !errors.Is(err, rlstest.ErrUnclassifiedRelation) {
			t.Fatalf("got %v, want ErrUnclassifiedRelation", err)
		}
		if !strings.Contains(err.Error(), "adoption_applications") {
			t.Fatalf("the failure does not name the offending table: %v", err)
		}
	})

	t.Run("reports every unclassified relation, not just the first", func(t *testing.T) {
		t.Parallel()

		err := c.ClassifyAll([]string{"pets", "documents", "media"})
		if err == nil {
			t.Fatal("accepted two unclassified relations")
		}
		for _, want := range []string{"documents", "media"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the failure hides %q, so fixing it takes one run per table: %v",
					want, err)
			}
		}
	})
}

// The anti-vacuity ledger.
//
// Until the last migration lands, the protection assertions run over a subset of
// the declared tables. Without this, a green suite would read as "19 tables are
// verified" while verifying none of them — the exact shape of the gaps mutation
// testing found in T-01-007 and T-01-008.
//
// Every migration task therefore has to delete its own lines from Pending, and
// the test fails in BOTH directions until it does.
func TestCheckPending(t *testing.T) {
	t.Parallel()

	c := rlstest.Classification{
		Tenant:         []string{"pets"},
		NonTenantModel: []string{"users"},
		Infrastructure: []string{"goose_db_version"},
		Pending:        map[string]string{"users": "T-01-013"},
	}

	t.Run("accepts a schema that matches the ledger", func(t *testing.T) {
		t.Parallel()

		if err := c.CheckPending([]string{"goose_db_version", "pets"}); err != nil {
			t.Fatalf("rejected a schema that matches the ledger exactly: %v", err)
		}
	})

	t.Run("a pending table that has landed is a failure", func(t *testing.T) {
		t.Parallel()

		err := c.CheckPending([]string{"goose_db_version", "pets", "users"})
		if !errors.Is(err, rlstest.ErrPendingTableExists) {
			t.Fatalf("got %v, want ErrPendingTableExists", err)
		}
		if !strings.Contains(err.Error(), "users") {
			t.Fatalf("the failure does not name the table: %v", err)
		}
	})

	t.Run("a declared table that is neither present nor pending is a failure", func(t *testing.T) {
		t.Parallel()

		err := c.CheckPending([]string{"goose_db_version"})
		if !errors.Is(err, rlstest.ErrTableMissing) {
			t.Fatalf("got %v, want ErrTableMissing", err)
		}
		if !strings.Contains(err.Error(), "pets") {
			t.Fatalf("the failure does not name the table: %v", err)
		}
	})
}

// The protection rule, exhaustive over every combination rather than inferred
// from whichever tables happen to exist on the day.
func TestCheckProtection(t *testing.T) {
	t.Parallel()

	c := rlstest.Classification{
		Tenant:         []string{"pets"},
		NonTenantModel: []string{"users", "refresh_tokens"},
		Infrastructure: []string{"goose_db_version"},
		NoPolicy:       []string{"refresh_tokens"},
	}

	cases := []struct {
		name    string
		table   rlstest.TableProtection
		wantErr error
		why     string
	}{
		{
			name:  "a fully protected tenant table passes",
			table: rlstest.TableProtection{Name: "pets", RLSEnabled: true, RLSForced: true, PolicyCount: 2},
		},
		{
			name: "the declared no-policy table passes with zero policies",
			table: rlstest.TableProtection{
				Name: "refresh_tokens", RLSEnabled: true, RLSForced: true, PolicyCount: 0,
			},
			why: "default-deny: RLS on with no policy denies everyone, which is stricter " +
				"than any policy this phase could write for it",
		},
		{
			name: "RLS not enabled is refused even with a policy",
			table: rlstest.TableProtection{
				Name: "pets", RLSEnabled: false, RLSForced: true, PolicyCount: 2,
			},
			wantErr: rlstest.ErrRLSNotEnabled,
			why: "a policy on a table whose RLS is not enabled is INERT; the table reads " +
				"as protected and is wide open",
		},
		{
			name: "RLS not forced is refused",
			table: rlstest.TableProtection{
				Name: "pets", RLSEnabled: true, RLSForced: false, PolicyCount: 2,
			},
			wantErr: rlstest.ErrRLSNotForced,
			why: "migrations run as the owner, and without FORCE the owner bypasses every " +
				"policy on its own tables",
		},
		{
			name: "a model table with no policy is refused",
			table: rlstest.TableProtection{
				Name: "users", RLSEnabled: true, RLSForced: true, PolicyCount: 0,
			},
			wantErr: rlstest.ErrNoPolicy,
			why: "users is protected ONLY by its EXISTS policy; without one it is " +
				"unreachable rather than isolated, and the omission would look deliberate",
		},
		{
			name: "an infrastructure relation is not required to be protected",
			table: rlstest.TableProtection{
				Name: "goose_db_version", RLSEnabled: false, RLSForced: false, PolicyCount: 0,
			},
			why: "goose creates it, not a migration; demanding RLS on it would mean " +
				"asserting something no migration in this repo controls",
		},
		{
			name: "an unclassified relation is refused rather than skipped",
			table: rlstest.TableProtection{
				Name: "ghosts", RLSEnabled: true, RLSForced: true, PolicyCount: 1,
			},
			wantErr: rlstest.ErrUnclassifiedRelation,
			why:     "silently passing an unknown table is precisely the gap being closed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := c.CheckProtection(tc.table)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("refused a correctly protected table (%s): %v", tc.why, err)
				}

				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v, want %v — %s", err, tc.wantErr, tc.why)
			}
			if !strings.Contains(err.Error(), tc.table.Name) {
				t.Errorf("the failure does not name the table: %v", err)
			}
		})
	}
}

// Everything above is pure. This is the half that reads a real catalog, and it
// is the one that fails the day a migration adds a table and forgets it here.
func TestCatalog_EveryRelationIsClassifiedAndProtected(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	tables, err := rlstest.ReadTables(ctx, env.OwnerPool)
	if err != nil {
		t.Fatalf("reading the catalog: %v", err)
	}
	if len(tables) == 0 {
		t.Fatal("the catalog reported no relations at all in schema public, so every " +
			"assertion below would be vacuous")
	}

	names := make([]string, 0, len(tables))
	for _, table := range tables {
		names = append(names, table.Name)
	}

	if err := rlstest.Schema.ClassifyAll(names); err != nil {
		t.Errorf("a relation in schema public escapes classification.\n"+
			"Add it to internal/db/rlstest/catalog.go — to Tenant if each of its rows "+
			"belongs to exactly one shelter, otherwise to NonTenantModel — and give it an "+
			"A/B case.\n%v", err)
	}

	if err := rlstest.Schema.CheckPending(names); err != nil {
		t.Errorf("the pending ledger no longer matches the schema.\n"+
			"A table listed as Pending has landed (delete its line, which switches its "+
			"protection assertions on) or a declared table is missing.\n%v", err)
	}

	checked := 0
	for _, table := range tables {
		if err := rlstest.Schema.CheckProtection(table); err != nil {
			t.Errorf("table is not protected: %v", err)
		}
		if rlstest.Schema.IsModelTable(table.Name) {
			checked++
		}
	}

	t.Logf("verified %d of %d declared model tables; %d still pending",
		checked, len(rlstest.Schema.ModelTables()), len(rlstest.Schema.Pending))
}

// The catalog reader has to see what a real migration produced, not only what a
// hand-written fixture claims. A table created here with the wrong flags must
// come back with those wrong flags, or the meta-test is reading nothing.
func TestReadTables_ReportsWhatTheCatalogActuallyHolds(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	dbtest.MustExec(t, env.OwnerPool, `CREATE TABLE catalog_probe (id uuid PRIMARY KEY)`)
	t.Cleanup(func() {
		_, _ = env.OwnerPool.Exec(ctx, `DROP TABLE IF EXISTS catalog_probe`)
	})

	find := func() rlstest.TableProtection {
		t.Helper()

		tables, err := rlstest.ReadTables(ctx, env.OwnerPool)
		if err != nil {
			t.Fatalf("reading the catalog: %v", err)
		}
		for _, table := range tables {
			if table.Name == "catalog_probe" {
				return table
			}
		}
		t.Fatal("the reader did not report a table that exists in schema public")

		return rlstest.TableProtection{}
	}

	probe := find()
	if probe.RLSEnabled || probe.RLSForced || probe.PolicyCount != 0 {
		t.Fatalf("a bare table came back as protected: %+v", probe)
	}

	dbtest.MustExec(t, env.OwnerPool, `ALTER TABLE catalog_probe ENABLE ROW LEVEL SECURITY`)
	dbtest.MustExec(t, env.OwnerPool, `ALTER TABLE catalog_probe FORCE ROW LEVEL SECURITY`)
	dbtest.MustExec(t, env.OwnerPool,
		`CREATE POLICY probe_policy ON catalog_probe FOR ALL TO app_tenant USING (true)`)

	probe = find()
	if !probe.RLSEnabled {
		t.Error("ENABLE ROW LEVEL SECURITY did not reach relrowsecurity")
	}
	if !probe.RLSForced {
		t.Error("FORCE ROW LEVEL SECURITY did not reach relforcerowsecurity")
	}
	if probe.PolicyCount != 1 {
		t.Errorf("policy count is %d, want 1", probe.PolicyCount)
	}
}

// An unclassified table must fail the suite. Proven against a real one rather
// than trusted, because this is the assertion the whole task exists for.
func TestCatalog_AnUnclassifiedTableFailsTheRule(t *testing.T) {
	env := dbtest.Postgres(t)
	ctx := context.Background()

	dbtest.MustExec(t, env.OwnerPool, `CREATE TABLE forgotten_table (id uuid PRIMARY KEY)`)
	t.Cleanup(func() {
		_, _ = env.OwnerPool.Exec(ctx, `DROP TABLE IF EXISTS forgotten_table`)
	})

	tables, err := rlstest.ReadTables(ctx, env.OwnerPool)
	if err != nil {
		t.Fatalf("reading the catalog: %v", err)
	}

	names := make([]string, 0, len(tables))
	for _, table := range tables {
		names = append(names, table.Name)
	}

	err = rlstest.Schema.ClassifyAll(names)
	if !errors.Is(err, rlstest.ErrUnclassifiedRelation) {
		t.Fatalf("a table nobody classified passed the rule: %v", err)
	}
	if !strings.Contains(err.Error(), "forgotten_table") {
		t.Fatalf("the failure does not name the table, so nobody could act on it: %v", err)
	}
}
