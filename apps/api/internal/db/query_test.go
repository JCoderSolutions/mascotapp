package db_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/gentleman/mascotapp/apps/api/internal/db/rlstest"
)

// queryFiles reads every `.sql` file in the sqlc queries directory, keyed by
// base name. It reads the directory from `sqlc.yaml` rather than hardcoding it,
// so moving the directory moves this test with it.
func queryFiles(t *testing.T) map[string]string {
	t.Helper()

	cfg := loadSQLCConfig(t)
	dir := filepath.Join(repoRoot(t), "apps", "api", filepath.FromSlash(cfg.SQL[0].Queries))

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the sqlc queries directory %s: %v", cfg.SQL[0].Queries, err)
	}

	files := map[string]string{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", entry.Name(), err)
		}
		files[entry.Name()] = string(body)
	}
	if len(files) == 0 {
		t.Fatalf("%s holds no .sql files", cfg.SQL[0].Queries)
	}

	return files
}

// The est on this task is, in the board's own words, *"the softest number in
// this file — no artifact states which queries Phase 01 owes, since handlers
// arrive in Phase 02"*. This is the rule that turns that judgement call into
// something checkable instead of leaving it to taste: **every declared model
// table is named by at least one query.**
//
// The point is not coverage for its own sake. `sqlc generate` type-checks every
// query against the MIGRATIONS — it reads the same `internal/db/migrations`
// directory goose applies — so a query is a compile-time assertion about the
// schema it was written against.
//
// HOW MUCH of an assertion depends on the query, and the difference was probed
// against sqlc v1.31.1 rather than assumed:
//
//   - a query that NAMES a column fails generation outright when a migration
//     removes it — `column "display_name" does not exist`, on the exact
//     file:line of the query;
//   - a `SELECT *` query does NOT. Generation succeeds and `models.go` comes
//     back with the new shape, so the failure moves to whoever reads the
//     removed field off the struct.
//
// Neither is silent, and that is the property worth having: one fails at
// generate time, the other at compile time. But it is worth stating plainly,
// because "sqlc catches schema drift" is a claim that is only half true, and the
// half that is true is the half where a query spells its columns out.
//
// So the minimum is one query per table, and this is what makes "minimum" a
// number rather than an opinion.
func TestQueries_ExerciseEveryDeclaredTable(t *testing.T) {
	t.Parallel()

	files := queryFiles(t)

	// COMMENTS STRIPPED FIRST, and that is not tidiness. Mutation removed
	// `pet_media`'s only query and `breeds`' only query, and this test stayed
	// green both times — because each table was still named in a COMMENT, in
	// this file set's own prose. A table nobody queries read as covered because
	// somebody had written its name in a sentence.
	//
	// It is the same failure the sqlc wiring guard had, one test earlier: asking
	// whether CI "mentions sqlc" was satisfied by a comment. A substring found
	// anywhere in a file is not an assertion about what the file DOES.
	var all strings.Builder
	for _, body := range files {
		all.WriteString(stripSQLComments(body))
		all.WriteString("\n")
	}
	corpus := all.String()

	// The one exemption, and it is DERIVED rather than hand-written: a table
	// declared default-deny has RLS on, no policy and no grant, so `app_tenant`
	// cannot reach it at all. A query against it would be code written to fail —
	// the same reason T-01-001 rejected a `//go:build tools` file and the sqlc
	// wiring guard rejected a placeholder query.
	//
	// Deriving it from `NoPolicy` is what makes the exemption expire on its own:
	// the day Phase 02 gives `refresh_tokens` an access path, it leaves that set
	// and this test starts demanding a query for it.
	exempt := map[string]string{}
	for _, table := range rlstest.Schema.NoPolicy {
		exempt[table] = "declared default-deny: no policy and no grant, so a query against " +
			"it could only ever fail"
	}

	var missing []string
	for _, table := range rlstest.Schema.ModelTables() {
		// Word-bounded, so `pets` does not match inside `pet_status_history`
		// and report a table nobody queried as covered.
		named := regexp.MustCompile(`\b` + regexp.QuoteMeta(table) + `\b`).MatchString(corpus)

		if why, isExempt := exempt[table]; isExempt {
			// The other direction, so the exemption cannot outlive its reason.
			if named {
				t.Errorf("a query names %q, which is %s. Either the table gained an access "+
					"path — in which case remove it from the NoPolicy declaration — or this "+
					"query cannot run", table, why)
			}

			continue
		}

		if !named {
			missing = append(missing, table)
		}
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("no query names %v. sqlc type-checks every query against the migrations, so "+
			"a table no query mentions gets no compile-time check at all: the day a later "+
			"migration renames one of its columns, nothing fails until Phase 02 writes a "+
			"handler. One query per table is the minimum that makes `sqlc generate` a real "+
			"gate", missing)
	}
}

// The convention that keeps the third layer of §3 honest: **a query does not
// filter by `shelter_id`. The policy does.**
//
// This looks like a style rule and it is a safety one. The whole argument for
// RLS is that *"un `WHERE shelter_id = ?` olvidado en una query es cuestión de
// tiempo"* — with RLS, that omission returns zero rows instead of another
// shelter's data. A query that filters by `shelter_id` ITSELF inverts that: it
// works identically whether or not the policy exists, so the day a policy is
// dropped, every such query keeps returning the right answer and the loss is
// invisible until someone writes a query that forgot.
//
// Belt-and-braces is the wrong instinct here. The belt has to be the layer that
// cannot be forgotten, and the braces must not be able to hide its absence.
//
// INSERTs are exempt and must be: `shelter_id` is NOT NULL on every tenant
// table, so a write SUPPLIES it — and `WITH CHECK` is what verifies the value.
// Supplying a column is not filtering by it.
func TestQueries_DoNotFilterByShelterID(t *testing.T) {
	t.Parallel()

	// `shelter_id` appearing after WHERE, AND, OR or a join condition — the
	// shapes that mean "filter", as opposed to naming the column in an INSERT's
	// column list or its VALUES.
	filtering := regexp.MustCompile(`(?i)(where|and|on)\s+[\w.]*shelter_id\s*=`)

	for name, body := range queryFiles(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if match := filtering.FindString(body); match != "" {
				t.Errorf("%s filters by shelter_id (%q). Tenant scoping is the POLICY's job "+
					"(§3 layer 3): a query that does it too returns the same rows whether or "+
					"not the policy exists, so dropping the policy would be invisible here "+
					"and would only surface through some other query that forgot. The layer "+
					"that cannot be forgotten has to be the one doing the work",
					name, strings.TrimSpace(match))
			}
		})
	}
}

// Every query carries a sqlc annotation, and the annotation carries a
// cardinality. `:one` and `:many` are different Go signatures, and picking the
// wrong one is a runtime error rather than a compile error — `:one` on a query
// that matches several rows returns the first and drops the rest silently.
//
// This does not check WHICH one is right. It checks that somebody chose.
func TestQueries_AreAllAnnotated(t *testing.T) {
	t.Parallel()

	annotation := regexp.MustCompile(`(?m)^-- name: (\w+) :(one|many|exec|execrows)$`)

	for name, body := range queryFiles(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			named := annotation.FindAllStringSubmatch(body, -1)
			if len(named) == 0 {
				t.Fatalf("%s contains no sqlc annotation, so sqlc generates nothing from it "+
					"and the file is SQL nobody type-checks", name)
			}

			// A malformed annotation is the failure this catches: sqlc skips
			// what it does not recognise, so a typo in `-- name:` produces a
			// file that generates nothing and reports no error.
			declared := strings.Count(body, "-- name:")
			if declared != len(named) {
				t.Errorf("%s has %d `-- name:` comments but only %d parse as sqlc "+
					"annotations. sqlc SKIPS what it cannot parse and says nothing, so the "+
					"difference is queries that silently generate no code", name, declared,
					len(named))
			}
		})
	}
}

// stripSQLComments removes `--` line comments, so a table named only in prose
// does not read as a table somebody queried.
//
// It deliberately does NOT handle `/* */` or comment markers inside string
// literals: this schema's queries use neither, and a half-correct SQL parser
// here would be a second thing to get wrong. The test below is what keeps that
// shortcut honest.
func stripSQLComments(body string) string {
	var out strings.Builder
	for _, line := range strings.Split(body, "\n") {
		if idx := strings.Index(line, "--"); idx >= 0 {
			line = line[:idx]
		}
		out.WriteString(line)
		out.WriteString("\n")
	}

	return out.String()
}

// The guard on that simplification. `stripSQLComments` handles `--` only, which
// is correct for every query in this repository and silently wrong the day one
// uses a block comment or puts `--` inside a string literal.
//
// Stating the limit as a test is what keeps a deliberate shortcut from becoming
// an accidental bug: the day it stops being adequate, this says so, instead of
// the coverage test quietly over-reporting the way it just did.
func TestQueries_UseOnlyTheCommentSyntaxTheStripperHandles(t *testing.T) {
	t.Parallel()

	for name, body := range queryFiles(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if strings.Contains(body, "/*") {
				t.Errorf("%s uses a block comment. stripSQLComments only removes `--` "+
					"lines, so the table-coverage test would count words inside it as "+
					"queried code", name)
			}

			for _, line := range strings.Split(body, "\n") {
				comment := strings.Index(line, "--")
				// An odd number of quotes before the marker means it sits INSIDE
				// a string literal, and stripping there would cut the statement
				// in half.
				if comment > 0 && strings.Count(line[:comment], "'")%2 == 1 {
					t.Errorf("%s has `--` inside a string literal (%q). stripSQLComments "+
						"would cut the literal in half and the table-coverage test would "+
						"stop seeing whatever follows", name, strings.TrimSpace(line))
				}
			}
		})
	}
}
