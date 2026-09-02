package db_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/gentleman/mascotapp/apps/api/internal/db"
)

// Build wiring is a contract like any other, and it fails the same way: quietly.
// A sqlc.yaml pointing at the wrong schema generates code for a schema nobody
// runs; a CI job pinning a different tool version than the devcontainer fails a
// diff check for a reason nobody can reproduce locally. Neither shows up in a
// test that only exercises Go code, so these assertions read the wiring files
// themselves.
//
// All of it is pure file reading, so it runs under -short and needs no Docker.

// repoRoot walks up from the test's working directory until it finds the
// Makefile. Counting `..` segments breaks the moment a file moves; looking for
// a landmark does not.
func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("locating the working directory: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "Makefile")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("walked to the filesystem root without finding the Makefile")
		}
		dir = parent
	}
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()

	body, err := os.ReadFile(filepath.Join(repoRoot(t), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("reading %s: %v", rel, err)
	}

	return string(body)
}

// sqlcConfig is the slice of sqlc.yaml this project's correctness depends on.
type sqlcConfig struct {
	Version string `yaml:"version"`
	SQL     []struct {
		Engine  string `yaml:"engine"`
		Schema  string `yaml:"schema"`
		Queries string `yaml:"queries"`
		Gen     struct {
			Go struct {
				Package       string `yaml:"package"`
				Out           string `yaml:"out"`
				SQLPackage    string `yaml:"sql_package"`
				EmitInterface bool   `yaml:"emit_interface"`
				Overrides     []struct {
					DBType string `yaml:"db_type"`
					GoType string `yaml:"go_type"`
				} `yaml:"overrides"`
			} `yaml:"go"`
		} `yaml:"gen"`
	} `yaml:"sql"`
}

func loadSQLCConfig(t *testing.T) sqlcConfig {
	t.Helper()

	var cfg sqlcConfig
	if err := yaml.Unmarshal([]byte(readRepoFile(t, "apps/api/sqlc.yaml")), &cfg); err != nil {
		t.Fatalf("parsing apps/api/sqlc.yaml: %v", err)
	}
	if len(cfg.SQL) != 1 {
		t.Fatalf("sqlc.yaml declares %d sql blocks, want exactly 1. More than one means "+
			"more than one generated package, and nothing here is ready for that",
			len(cfg.SQL))
	}

	return cfg
}

func TestSQLCConfig_MatchesTheDesignedContract(t *testing.T) {
	t.Parallel()

	cfg := loadSQLCConfig(t)
	block := cfg.SQL[0]
	gen := block.Gen.Go

	for _, tc := range []struct {
		what string
		got  string
		want string
		why  string
	}{
		{"version", cfg.Version, "2", "v1 has a different, incompatible key layout"},
		{"engine", block.Engine, "postgresql", "the whole isolation model is PostgreSQL RLS"},
		{
			"schema", block.Schema, "internal/db/migrations",
			"sqlc must read the SAME SQL goose applies. A separate schema file is a " +
				"second source of truth that drifts silently",
		},
		{"queries", block.Queries, "internal/db/query", "design §Files"},
		{"gen.go.out", gen.Out, "internal/db/sqlcgen", "design §Files"},
		{"gen.go.package", gen.Package, "sqlcgen", "design §Files"},
		{
			"gen.go.sql_package", gen.SQLPackage, "pgx/v5",
			"WithTenant hands the queries a pgx.Tx; database/sql output would not compile " +
				"against it, and the tenant scope lives on that transaction",
		},
	} {
		if tc.got != tc.want {
			t.Errorf("sqlc.yaml %s = %q, want %q — %s", tc.what, tc.got, tc.want, tc.why)
		}
	}

	if !gen.EmitInterface {
		t.Error("sqlc.yaml gen.go.emit_interface is false. The Querier interface is what " +
			"lets a handler take a dependency it can substitute in a test")
	}

	var uuidOverride string
	for _, override := range gen.Overrides {
		if override.DBType == "uuid" {
			uuidOverride = override.GoType
		}
	}
	if uuidOverride != "github.com/google/uuid.UUID" {
		t.Errorf("sqlc.yaml maps db_type uuid to %q, want github.com/google/uuid.UUID. "+
			"Every primary key in this schema is a uuid and WithTenant already takes a "+
			"uuid.UUID; without the override the generated code speaks pgtype.UUID and "+
			"every call site converts", uuidOverride)
	}
}

// sqlc reads the migration directory from disk; the application runs the set
// that //go:embed captured. That is two readers of what is meant to be one
// schema, and the Makefile targets added in T-01-012 make the disk copy
// something a human actually runs.
//
// The gap is narrow and real: a file the embed pattern does not match — a
// subdirectory, an uppercase .SQL extension — is invisible to every existing
// invariant, because they all read the embedded set.
func TestMigrations_OnDiskSetMatchesTheEmbeddedSet(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(filepath.Join(repoRoot(t), "apps", "api", "internal", "db", "migrations"))
	if err != nil {
		t.Fatalf("reading the migrations directory: %v", err)
	}

	var onDisk []string
	for _, entry := range entries {
		if entry.IsDir() {
			t.Errorf("%s is a directory. `//go:embed migrations/*.sql` does not descend, "+
				"so anything inside it would never ship", entry.Name())

			continue
		}
		onDisk = append(onDisk, entry.Name())
	}
	sort.Strings(onDisk)

	embeddedEntries, err := fs.ReadDir(db.Migrations(), ".")
	if err != nil {
		t.Fatalf("reading the embedded set: %v", err)
	}
	var embedded []string
	for _, entry := range embeddedEntries {
		embedded = append(embedded, entry.Name())
	}
	sort.Strings(embedded)

	if strings.Join(onDisk, ",") != strings.Join(embedded, ",") {
		t.Errorf("the migration directory and the embedded set disagree.\n"+
			"  on disk:  %v\n  embedded: %v\n"+
			"sqlc and `make migrate` read the directory; the application runs the embed. "+
			"A file in one and not the other is a migration that generates code but never "+
			"runs, or runs but was never reviewed as part of the schema", onDisk, embedded)
	}
}

// makeTarget returns the recipe lines of one Makefile target.
func makeTarget(t *testing.T, makefile, name string) string {
	t.Helper()

	lines := strings.Split(makefile, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, name+":") {
			continue
		}
		var recipe []string
		for _, next := range lines[i+1:] {
			if strings.TrimSpace(next) == "" || strings.HasPrefix(next, "\t") ||
				strings.HasPrefix(next, "#") {
				recipe = append(recipe, next)

				continue
			}

			break
		}

		return strings.Join(recipe, "\n")
	}
	t.Fatalf("the Makefile has no %q target", name)

	return ""
}

func TestMakefile_HasTheDatabaseTargets(t *testing.T) {
	t.Parallel()

	makefile := readRepoFile(t, "Makefile")

	phony := ""
	for _, line := range strings.Split(makefile, "\n") {
		if strings.HasPrefix(line, ".PHONY:") {
			phony = line
		}
	}

	for _, target := range []string{"migrate", "migrate-down", "db-reset"} {
		t.Run(target, func(t *testing.T) {
			if !strings.Contains(makefile, "\n"+target+":") {
				t.Fatalf("the Makefile has no %q target", target)
			}
			// Without .PHONY, a file or directory of the same name makes the
			// target a silent no-op.
			if !regexp.MustCompile(`\b` + regexp.QuoteMeta(target) + `\b`).MatchString(phony) {
				t.Errorf("%q is missing from .PHONY", target)
			}
		})
	}
}

// The DoD: `make db-reset` owns local teardown. Agents are denied `rm` outright
// by .claude/settings.json precisely so that every destructive path is a
// reviewed, versioned artifact rather than a model's judgment call — and a
// reset target that quietly shells out to `rm` would put that judgment back.
func TestMakefile_DbResetDestroysNothingByHand(t *testing.T) {
	t.Parallel()

	recipe := makeTarget(t, readRepoFile(t, "Makefile"), "db-reset")

	for _, forbidden := range []string{"rm ", "rm -", "docker volume rm", "docker compose down -v"} {
		if strings.Contains(recipe, forbidden) {
			t.Errorf("db-reset runs %q. Teardown must go through the migration set, which "+
				"is reviewed SQL, rather than through a filesystem or volume deletion whose "+
				"blast radius is whatever the path happens to expand to", forbidden)
		}
	}

	// A reset both destroys and rebuilds. `migrate-down` is deliberately ONE
	// step, so a reset cannot be built from it — "all the way down" is
	// `down-to 0`, and the bring-back-up half is what separates a reset from a
	// teardown wearing a reset's name.
	if !strings.Contains(recipe, "down-to 0") {
		t.Error("db-reset does not roll all the way down. `migrate-down` is a single step " +
			"and cannot express a reset")
	}
	if !strings.Contains(recipe, "migrate") {
		t.Error("db-reset tears down without bringing the schema back, which leaves the " +
			"developer with no schema and no message saying so")
	}
}

// A target that runs a migration against whatever DATABASE_URL happens to hold
// is one shell export away from migrating production. Requiring it explicitly,
// and failing loudly when it is unset, is the difference between a mistake and
// an accident.
func TestMakefile_MigrationTargetsRefuseAnUnsetDSN(t *testing.T) {
	t.Parallel()

	makefile := readRepoFile(t, "Makefile")

	for _, target := range []string{"migrate", "migrate-down"} {
		t.Run(target, func(t *testing.T) {
			recipe := makeTarget(t, makefile, target)
			if !strings.Contains(recipe, "DATABASE_URL") {
				t.Fatalf("%s does not mention DATABASE_URL", target)
			}
			// A default value is the failure mode: it makes the target work
			// without thinking, against whatever database the default names.
			if strings.Contains(makefile, "DATABASE_URL ?=") ||
				strings.Contains(makefile, "DATABASE_URL :=") ||
				strings.Contains(makefile, "DATABASE_URL=") {
				t.Errorf("the Makefile defaults DATABASE_URL. A migration target with a " +
					"default DSN runs somewhere whether or not anyone chose it")
			}
		})
	}
}

// The deferral guard.
//
// `sqlc generate` FAILS with an empty queries directory — verified 2026-08-30
// against sqlc v1.31.1: "error parsing queries: no queries contained in paths".
// The first query needs the first table, which arrives in T-01-013, so wiring
// sqlc into `make generate` today would break the build for twenty-one tasks.
//
// Writing a placeholder query to keep a target green would be code that lies
// about why it exists — the same reason a `//go:build tools` file was rejected
// in T-01-001. So the wiring is deferred, and this makes the deferral impossible
// to forget: the moment a real query file lands, this test goes red until
// `make generate` and CI regenerate and diff-check it.
func TestSQLC_IsWiredIntoGenerateExactlyWhenQueriesExist(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	cfg := loadSQLCConfig(t)
	queryDir := filepath.Join(root, "apps", "api", filepath.FromSlash(cfg.SQL[0].Queries))

	var queries []string
	entries, err := os.ReadDir(queryDir)
	switch {
	case os.IsNotExist(err):
		// Not yet created; the same as empty for this rule.
	case err != nil:
		t.Fatalf("reading %s: %v", cfg.SQL[0].Queries, err)
	default:
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".sql") {
				queries = append(queries, entry.Name())
			}
		}
	}

	generate := makeTarget(t, readRepoFile(t, "Makefile"), "generate")
	inMakefile := strings.Contains(generate, "sqlc generate")

	// Both halves, and separately, because mutation showed that asking whether
	// the CI file MENTIONS sqlc is not an assertion: a comment satisfies it. A
	// workflow that installs the tool and never runs it, or runs it and never
	// compares the result, passes a substring check and gates nothing.
	//
	// The spec's requirement is the DIFF — *"CI MUST fail when regeneration
	// produces a diff"* — so both the regeneration and the comparison have to be
	// there for this to mean what it says.
	ci := readRepoFile(t, ".github/workflows/ci.yml")
	regenerates := strings.Contains(ci, "sqlc generate")
	diffChecks := strings.Contains(ci, "git diff --exit-code -- internal/db/sqlcgen")
	inCI := regenerates && diffChecks

	inDevcontainer := strings.Contains(readRepoFile(t, ".devcontainer/postCreate.sh"), "sqlc")

	if len(queries) == 0 {
		if inMakefile {
			t.Error("`make generate` runs sqlc but there are no query files. sqlc exits 1 " +
				"on an empty queries directory, so this breaks the build for everyone")
		}

		return
	}

	if !inMakefile {
		t.Errorf("query files exist (%v) but `make generate` does not run sqlc. The "+
			"generated code would be whatever was committed last, not what the queries say",
			queries)
	}
	if !inCI {
		t.Errorf("query files exist (%v) but CI does not both regenerate (%v) and "+
			"diff-check (%v) the sqlc output. The spec requires CI to FAIL when "+
			"regeneration produces a diff, so the SQL under review is the SQL that runs — "+
			"and a step that regenerates without comparing is a step that proves nothing",
			queries, regenerates, diffChecks)
	}
	if !inDevcontainer {
		t.Errorf("query files exist (%v) but the devcontainer does not install sqlc, so "+
			"`make generate` fails on a fresh container", queries)
	}
}

// A Makefile target that shells out to a tool nobody installs works on the
// machine that wrote it and fails on every fresh devcontainer. T-01-012 added
// three targets that invoke the goose CLI, so the rule is worth stating rather
// than assuming.
//
// It reads the Makefile rather than a hardcoded list, so a target added later
// brings its own requirement with it.
func TestDevcontainerInstallsEveryToolTheMakefileInvokes(t *testing.T) {
	t.Parallel()

	// Available without installation: shell builtins, coreutils, and the
	// language runtimes the devcontainer image already provides.
	ambient := map[string]bool{
		"cd": true, "rm": true, "mkdir": true, "echo": true, "if": true, "fi": true,
		"then": true, "else": true, "exit": true, "make": true, "docker": true,
		"go": true, "npm": true, "gofmt": true, "unformatted": true, "for": true,
		"done": true, "set": true, "true": true, "false": true,
	}

	makefile := readRepoFile(t, "Makefile")
	postCreate := readRepoFile(t, ".devcontainer/postCreate.sh")

	// The binary a `go install <module>/cmd/<name>@<ver>` line provides is the
	// last path segment of the module path.
	installed := map[string]bool{}
	for _, match := range regexp.MustCompile(`go install\s+([^\s@]+)@`).
		FindAllStringSubmatch(postCreate, -1) {
		parts := strings.Split(match[1], "/")
		installed[parts[len(parts)-1]] = true
	}

	invoked := map[string]bool{}
	for _, line := range recipeLines(makefile) {
		for _, command := range strings.Split(strings.TrimPrefix(strings.TrimSpace(line), "@"), "&&") {
			fields := strings.Fields(strings.TrimSpace(command))
			// `GOTMPDIR=... go test` and `CGO_ENABLED=1 go test` put env
			// assignments before the command; the command is the first field
			// that is not one.
			for len(fields) > 0 && strings.Contains(fields[0], "=") {
				fields = fields[1:]
			}
			if len(fields) == 0 {
				continue
			}
			name := fields[0]
			if strings.HasPrefix(name, "$") || strings.HasPrefix(name, "\"") {
				continue
			}
			if !ambient[name] {
				invoked[name] = true
			}
		}
	}

	if len(invoked) == 0 {
		t.Fatal("no external tools were found in any Makefile recipe, so this test would " +
			"compare nothing")
	}

	for name := range invoked {
		if !installed[name] {
			t.Errorf("a Makefile recipe runs %q but .devcontainer/postCreate.sh never "+
				"installs it. The target works wherever it was written and fails on every "+
				"fresh container", name)
		}
	}
	t.Logf("checked %d tool(s) invoked by the Makefile", len(invoked))
}

// recipeLines returns each Makefile recipe as ONE logical line, joining
// backslash continuations.
//
// The reader used to take every tab-indented PHYSICAL line as its own command,
// which is not what a Makefile means. A continued recipe such as
//
//	target:
//		docker run --rm \
//			-v /a:/b \
//			golang:1.27 go test ./...
//
// is a single `docker` invocation; read line by line it becomes three commands
// whose names are `docker`, `-v` and `golang:1.27`. The check then demands that
// postCreate.sh install a tool called `-v`, which is nonsense that reads exactly
// like a real finding.
//
// Found on 2026-08-31 by `test-api-container`, the first continued recipe this
// Makefile has ever had. The check itself is unchanged and still fails on a
// genuinely missing tool — only its idea of where a command ends is fixed.
func recipeLines(makefile string) []string {
	var out []string
	var pending strings.Builder

	for _, line := range strings.Split(makefile, "\n") {
		if !strings.HasPrefix(line, "\t") && pending.Len() == 0 {
			continue
		}

		trimmed := strings.TrimRight(line, " \t\r")
		if strings.HasSuffix(trimmed, `\`) {
			pending.WriteString(strings.TrimSuffix(trimmed, `\`))
			pending.WriteString(" ")

			continue
		}

		pending.WriteString(trimmed)
		out = append(out, "\t"+strings.TrimSpace(pending.String()))
		pending.Reset()
	}

	if pending.Len() > 0 {
		out = append(out, "\t"+strings.TrimSpace(pending.String()))
	}

	return out
}

// A generated-code diff check is only meaningful if everyone generates with the
// same tool version. Pinning in two files with no check between them is how the
// devcontainer and CI drift, and the symptom is a CI diff nobody can reproduce.
func TestPinnedToolVersionsAgreeBetweenDevcontainerAndCI(t *testing.T) {
	t.Parallel()

	// `go install <module>/<path>@<version>`, capturing module and version.
	install := regexp.MustCompile(`go install\s+([^\s@]+)@([^\s"']+)`)

	pins := func(body string) map[string]string {
		out := map[string]string{}
		for _, match := range install.FindAllStringSubmatch(body, -1) {
			out[match[1]] = match[2]
		}

		return out
	}

	devcontainer := pins(readRepoFile(t, ".devcontainer/postCreate.sh"))
	ci := pins(readRepoFile(t, ".github/workflows/ci.yml"))

	if len(devcontainer) == 0 {
		t.Fatal("no pinned tool installs found in .devcontainer/postCreate.sh, so this " +
			"test would compare nothing")
	}

	shared := 0
	for module, want := range devcontainer {
		got, ok := ci[module]
		if !ok {
			continue // installed in one place only; nothing to disagree about
		}
		shared++
		if got != want {
			t.Errorf("%s is pinned to %s in the devcontainer and %s in CI. Generated code "+
				"differs between tool versions, so the diff check would fail for a reason "+
				"nobody can reproduce locally", module, want, got)
		}
	}
	t.Logf("compared %d tool(s) installed in both places", shared)
}
