package db_test

import (
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman/mascotapp/apps/api/internal/db"
)

// Every invariant here is checked against the EMBEDDED filesystem, not against
// the working directory. What ships in the binary is what runs against Neon; a
// file that exists on disk but was never embedded is a migration that silently
// does not happen in production.
//
// All of it is pure Go over an fs.FS, so it runs under -short and needs no
// Docker daemon.

var migrationName = regexp.MustCompile(`^(\d{5})_[a-z0-9_]+\.sql$`)

type migration struct {
	name    string
	version int
	body    string
}

func loadMigrations(t *testing.T) []migration {
	t.Helper()

	entries, err := fs.ReadDir(db.Migrations(), ".")
	if err != nil {
		t.Fatalf("reading the embedded migration set: %v", err)
	}

	var out []migration
	for _, entry := range entries {
		if entry.IsDir() {
			t.Errorf("%s is a directory; the migration set is flat", entry.Name())

			continue
		}

		body, err := fs.ReadFile(db.Migrations(), entry.Name())
		if err != nil {
			t.Fatalf("reading %s: %v", entry.Name(), err)
		}

		match := migrationName.FindStringSubmatch(entry.Name())
		if match == nil {
			t.Errorf("%s does not match NNNNN_lower_snake_case.sql", entry.Name())

			continue
		}
		version, err := strconv.Atoi(match[1])
		if err != nil {
			t.Fatalf("parsing the version of %s: %v", entry.Name(), err)
		}

		out = append(out, migration{name: entry.Name(), version: version, body: string(body)})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })

	return out
}

// An empty embedded set is the quietest possible failure: `goose up` succeeds,
// reports nothing to do, and the application starts against a database with no
// schema, no roles and no policies.
func TestMigrations_EmbeddedSetIsNotEmpty(t *testing.T) {
	t.Parallel()

	if len(loadMigrations(t)) == 0 {
		t.Fatal("the embedded migration set is empty: `goose up` would succeed and do nothing")
	}
}

// goose orders by version. A duplicate makes the order between two files
// undefined, and a gap almost always means a migration was written and never
// embedded — both of which show up as a schema that differs between machines.
func TestMigrations_VersionsAreStrictlyAscendingFromOneWithNoGaps(t *testing.T) {
	t.Parallel()

	migrations := loadMigrations(t)
	for i, m := range migrations {
		want := i + 1
		if m.version != want {
			t.Errorf("%s has version %d, expected %d: versions must run 1..N with no gaps "+
				"and no duplicates", m.name, m.version, want)
		}
	}
}

// A migration with no Down cannot be rolled back, so `goose down-to` stops at
// it. The rollback plan in proposal.md depends on every step being reversible.
func TestMigrations_EveryUpHasADown(t *testing.T) {
	t.Parallel()

	for _, m := range loadMigrations(t) {
		if !strings.Contains(m.body, "+goose Up") {
			t.Errorf("%s has no `-- +goose Up` marker", m.name)
		}
		if !strings.Contains(m.body, "+goose Down") {
			t.Errorf("%s has no `-- +goose Down`: it could never be rolled back", m.name)
		}
	}
}

// Design decision D8: role passwords never appear in migration text. ALTER ROLE
// ... PASSWORD is a utility statement and cannot take bind parameters — the same
// trap as SET LOCAL — so putting it in a migration forces the secret into a file
// that is committed, embedded in the binary, and printed by `goose status`.
func TestMigrations_ContainNoPasswordLiteral(t *testing.T) {
	t.Parallel()

	for _, m := range loadMigrations(t) {
		for i, line := range strings.Split(m.body, "\n") {
			if hasPasswordClause(line) {
				t.Errorf("%s:%d contains a PASSWORD clause, which D8 forbids: %q",
					m.name, i+1, strings.TrimSpace(line))
			}
		}
	}
}

// passwordClause matches PASSWORD as a SQL keyword, on a word boundary.
//
// It was a plain substring match until 00002 introduced `users.password_hash`
// and the guard fired on a column name. Widening a security rule until it
// catches innocent text is how a rule gets deleted; `_` is a word character, so
// this matches `ALTER ROLE ... PASSWORD 'x'` and not `password_hash`, which is
// exactly the distinction D8 is about.
var passwordClause = regexp.MustCompile(`(?i)\bpassword\b`)

// hasPasswordClause reports whether a line of migration SQL carries one. A
// comment may legitimately explain why there is no password here.
func hasPasswordClause(line string) bool {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "--") {
		return false
	}

	return passwordClause.MatchString(trimmed)
}

// The rule above was loosened, so what it still catches is asserted rather than
// assumed. Every "must be caught" line below is a real way the secret ends up in
// a committed file.
func TestHasPasswordClause(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		line string
		want bool
	}{
		{`ALTER ROLE app_tenant PASSWORD 'hunter2';`, true},
		{`CREATE ROLE app_tenant LOGIN PASSWORD 'hunter2';`, true},
		{`    alter role app_public password 'x';`, true},
		{`ALTER ROLE app_tenant PASSWORD NULL;`, true},
		{`-- Nothing here sets a password. See D8.`, false},
		{`    password_hash     text,`, false},
		{`    totp_secret_enc   bytea,`, false},
		{`CREATE TABLE users (id uuid NOT NULL PRIMARY KEY);`, false},
	} {
		if got := hasPasswordClause(tc.line); got != tc.want {
			t.Errorf("hasPasswordClause(%q) = %v, want %v", tc.line, got, tc.want)
		}
	}
}

// Roles must exist before any table enables row-level security. A policy names
// the role it applies TO, so a table that enables RLS before app_tenant exists
// either fails to migrate or, worse, ends up with RLS on and no policy for a
// role that arrives later.
func TestMigrations_RolesAreCreatedBeforeAnyTableEnablesRLS(t *testing.T) {
	t.Parallel()

	migrations := loadMigrations(t)

	rolesVersion := -1
	firstRLSVersion := -1
	for _, m := range migrations {
		upper := strings.ToUpper(m.body)
		if rolesVersion == -1 && strings.Contains(upper, "CREATE ROLE") {
			rolesVersion = m.version
		}
		if firstRLSVersion == -1 && strings.Contains(upper, "ENABLE ROW LEVEL SECURITY") {
			firstRLSVersion = m.version
		}
	}

	if rolesVersion == -1 {
		t.Fatal("no migration creates a role: the application would connect as the owner, " +
			"and RLS does not apply to the owner")
	}
	if firstRLSVersion == -1 {
		// Nothing enables RLS yet, so there is no ordering to violate.
		return
	}
	if rolesVersion >= firstRLSVersion {
		t.Fatalf("roles are created in migration %05d but RLS is first enabled in %05d: "+
			"roles must come strictly first", rolesVersion, firstRLSVersion)
	}
}

// FORCE is not optional and not a duplicate of ENABLE. Without it the table
// OWNER bypasses every policy, which is precisely the classic mistake ADR-0002
// exists to prevent — and migrations run as the owner.
func TestMigrations_EveryTableThatEnablesRLSAlsoForcesIt(t *testing.T) {
	t.Parallel()

	enable := regexp.MustCompile(`(?i)ALTER\s+TABLE\s+(\w+)\s+ENABLE\s+ROW\s+LEVEL\s+SECURITY`)
	force := regexp.MustCompile(`(?i)ALTER\s+TABLE\s+(\w+)\s+FORCE\s+ROW\s+LEVEL\s+SECURITY`)

	enabled := map[string]string{}
	forced := map[string]bool{}
	for _, m := range loadMigrations(t) {
		for _, match := range enable.FindAllStringSubmatch(m.body, -1) {
			enabled[strings.ToLower(match[1])] = m.name
		}
		for _, match := range force.FindAllStringSubmatch(m.body, -1) {
			forced[strings.ToLower(match[1])] = true
		}
	}

	for table, file := range enabled {
		if !forced[table] {
			t.Errorf("%s enables RLS on %q but never FORCEs it: the owner would bypass "+
				"every policy on that table", file, table)
		}
	}
}
