//go:build ignore

// Command doctor is the session-start preflight for MascotApp.
//
// It answers one question nothing else in this repository answers: is every
// external tool this workflow depends on actually PRESENT AND RUNNABLE on THIS
// machine, right now?
//
// Why it exists. On 2026-09-02 `gentle-ai` disappeared from this host in the
// middle of a session and nothing broke loudly. The step that needed it ran
// inside a pipeline; its `command not found` went to stderr, the pipeline
// returned 0, and the session carried on believing the SDD ledger had been
// updated. A missing tool has to fail at the START of a session, in the
// foreground, not silently in the middle of one.
//
// Three rules, each one a lesson this project has already paid for:
//
//  1. It EXECUTES every tool instead of only looking it up on PATH. Presence is
//     not runnability: on this Windows host, Smart App Control lets a binary sit
//     on disk and refuses to run it.
//  2. `make doctor` runs it with NO PIPE. A pipeline returns the status of its
//     LAST command, which is how a red test run was once reported as green.
//  3. Its manifest is bound to the Makefile. If a recipe starts invoking a tool
//     the manifest does not know about, doctor reports that as a finding against
//     ITSELF rather than going quietly out of date.
//
// Exit status is non-zero only when a REQUIRED tool is missing or unrunnable.
// An optional tool that is gone is reported in full and does not fail the run --
// the point is that you SEE it, not that you are blocked by it.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// probeTimeout bounds every external command. `docker version` against a daemon
// that is installed but not running is the case that would otherwise hang a
// preflight that exists to be fast.
const probeTimeout = 20 * time.Second

type severity int

const (
	required severity = iota
	optional
)

type check struct {
	name  string
	sev   severity
	probe []string // argv; argv[0] is the binary looked up on PATH
	why   string   // what this tool is for here
	gone  string   // what stops working without it
}

// manifest is the list of tools this workflow depends on, with what each one
// costs when it is missing. It is written out rather than derived because the
// value of this program is the SECOND column: a name alone tells you nothing
// about whether you can keep working.
//
// `make`, shell builtins and coreutils are deliberately absent: if they were
// missing you would not have reached this program.
var manifest = []check{
	{
		name: "go", sev: required,
		probe: []string{"go", "version"},
		why:   "the API toolchain: build, test, vet, and every `go run` recipe",
		gone:  "nothing in apps/api builds or runs",
	},
	{
		name: "docker", sev: required,
		// --format reaches the SERVER, so this fails when the CLI is installed
		// and the daemon is down. That distinction is the whole point: on this
		// host `test-api-container` is the only way to run the suite at all.
		probe: []string{"docker", "version", "--format", "{{.Server.Version}}"},
		why:   "runs the test suite (test-api-container) and the local stack (dev)",
		gone:  "no test run of any kind is possible on this host; `make test-short` proves nothing about isolation",
	},
	{
		name: "git", sev: required,
		probe: []string{"git", "--version"},
		why:   "branch-per-PR delivery and every commit in the task-close ritual",
		gone:  "no commits, no branches, no recovery point",
	},
	{
		name: "npm", sev: required,
		probe: []string{"npm", "--version"},
		why:   "apps/web: dependencies, test, lint, typecheck, type generation",
		gone:  "the web side of `make test`, `make lint` and `make generate` fails",
	},
	{
		name: "golangci-lint", sev: required,
		probe: []string{"golangci-lint", "--version"},
		why:   "the Go linter in `make lint-api`, part of every task's definition of done",
		gone:  "no task can be closed: lint clean is not verifiable",
	},
	{
		name: "govulncheck", sev: required,
		probe: []string{"govulncheck", "-version"},
		why:   "supply-chain gate in `make lint-api` and in CI",
		gone:  "vulnerable dependencies stop being detected before they reach CI",
	},
	{
		name: "sqlc", sev: required,
		probe: []string{"sqlc", "version"},
		why:   "generates the query layer from internal/db/query + the migrations",
		gone:  "`make generate` fails; new queries cannot reach Go code",
	},
	{
		name: "oapi-codegen", sev: required,
		probe: []string{"oapi-codegen", "--version"},
		why:   "generates the HTTP server types from api/openapi.yaml",
		gone:  "`make generate` fails; the API contract and the code drift apart",
	},
	{
		name: "goose", sev: optional,
		probe: []string{"goose", "--version"},
		why:   "the migration CLI behind `make migrate`, `migrate-down` and `db-reset`",
		gone:  "no migrations against a real database from this host. The test suite is unaffected: it embeds the same migration set and applies it itself",
	},
	{
		name: "gentle-ai", sev: optional,
		probe: []string{"gentle-ai", "--version"},
		why:   "the SDD ledger: attempt accounting, sdd-status/continue, and the review lifecycle",
		gone:  "the SDD attempt ledger cannot be settled. The WORK does not depend on it -- TDD, the test suite, mutation testing, per-PR budgets and commits all keep working. What is lost is the bookkeeping, and an unsettled attempt has to be recorded in PROJECT_STATE.md by hand",
	},
	// NOT checked, on purpose: `rg`. Agents are told to prefer it over grep, so
	// it looks like it belongs here -- and the first run of this program proved
	// otherwise. `rg` is a SHELL FUNCTION that Claude Code injects, proxying to
	// the ripgrep bundled inside its own binary; there is no `rg` on PATH and
	// there never will be, so this check would be red forever on a healthy
	// machine. A preflight with a permanent false alarm teaches its reader to
	// skim past WARN lines, which is the exact habit this program exists to
	// break. Only check things whose absence actually means something.
}

type result struct {
	check
	status  string // OK, WARN, FAIL
	detail  string
	failing bool
}

func main() {
	root, err := repoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("MascotApp preflight -- %s/%s\n\n", runtime.GOOS, runtime.GOARCH)

	results := make([]result, 0, len(manifest)+1)
	for _, c := range manifest {
		results = append(results, run(c))
	}
	results = append(results, freshBinaryCheck())

	width := 0
	for _, r := range results {
		if len(r.name) > width {
			width = len(r.name)
		}
	}

	var problems []result
	for _, r := range results {
		fmt.Printf("  %-4s %-*s  %s\n", r.status, width, r.name, r.detail)
		if r.status != "OK" {
			problems = append(problems, r)
		}
	}

	unknown := unknownMakefileTools(root)
	blocked := false

	if len(problems) > 0 {
		fmt.Println()
		for _, r := range problems {
			fmt.Printf("%s -- %s\n", r.name, r.why)
			fmt.Printf("  without it: %s\n\n", r.gone)
			if r.failing {
				blocked = true
			}
		}
	}

	if len(unknown) > 0 {
		blocked = true
		fmt.Println("This preflight is out of date:")
		for _, name := range unknown {
			fmt.Printf("  a Makefile recipe runs %q and doctor's manifest does not list it.\n", name)
		}
		fmt.Println("  Add it to `manifest` in scripts/doctor.go with what breaks without it,")
		fmt.Println("  or the next person to lose that tool finds out the expensive way.")
		fmt.Println()
	}

	switch {
	case blocked:
		fmt.Println("Preflight FAILED. Fix the above before starting work.")
		os.Exit(1)
	case len(problems) > 0:
		fmt.Println("Preflight passed with warnings. Nothing above blocks the build --")
		fmt.Println("read what each one costs and decide, but decide knowingly.")
	default:
		fmt.Println("Preflight clean.")
	}
}

// run executes the probe. It separates three outcomes that a plain lookup
// collapses into one: not on PATH, on PATH but the process would not start, and
// on PATH and it ran.
func run(c check) result {
	res := result{check: c}

	path, err := exec.LookPath(c.probe[0])
	if err != nil {
		res.status = "FAIL"
		res.detail = "not on PATH"
		res.failing = c.sev == required
		if c.sev == optional {
			res.status = "WARN"
		}
		return res
	}

	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, path, c.probe[1:]...).CombinedOutput()
	if ctx.Err() != nil {
		res.status = "FAIL"
		res.detail = fmt.Sprintf("found, but %q did not answer in %s", strings.Join(c.probe, " "), probeTimeout)
		res.failing = c.sev == required
		if c.sev == optional {
			res.status = "WARN"
		}
		return res
	}
	if err != nil {
		res.status = "FAIL"
		res.detail = fmt.Sprintf("found at %s, but it would not run: %v (%s)", path, err, firstLine(out))
		res.failing = c.sev == required
		if c.sev == optional {
			res.status = "WARN"
		}
		return res
	}

	res.status = "OK"
	res.detail = firstLine(out)
	return res
}

// freshBinaryCheck compiles and runs a throwaway program.
//
// It is not a tool check, it is a HOST check, and it is here because it is the
// single most expensive thing this project has learned. On this Windows machine
// Smart App Control refuses to execute freshly linked unsigned binaries, so
// `go test` never runs a single test and exits non-zero -- which reads exactly
// like a failing suite. Three separate confident diagnoses were wrong before it
// was identified (see the comment above `test-api-container` in the Makefile).
//
// Two seconds here names it up front instead of after an afternoon.
func freshBinaryCheck() result {
	c := check{
		name: "fresh-binary", sev: optional,
		why:  "proves this host can EXECUTE a newly compiled binary, which is what `go test` does on every run",
		gone: "`go test` exits non-zero without running any test, and the output reads like a failing suite. This is Smart App Control on Windows and it has no exclusion list. The runner is `make test-api-container`, which links and runs inside Linux",
	}
	res := result{check: c}

	dir, err := os.MkdirTemp("", "mascotapp-doctor-")
	if err != nil {
		res.status = "WARN"
		res.detail = "could not create a scratch directory: " + err.Error()
		return res
	}
	defer os.RemoveAll(dir)

	const token = "mascotapp-doctor-ok"
	files := map[string]string{
		"go.mod":  "module doctorprobe\n\ngo 1.21\n",
		"main.go": "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Print(\"" + token + "\") }\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			res.status = "WARN"
			res.detail = "could not write the probe: " + err.Error()
			return res
		}
	}

	binary := filepath.Join(dir, "probe")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}

	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	build := exec.CommandContext(ctx, "go", "build", "-o", binary, ".")
	build.Dir = dir
	// GOFLAGS/GOPROXY from the caller must not reach a module with no
	// dependencies; GOPROXY=off is enough and keeps the probe offline.
	build.Env = append(os.Environ(), "GOFLAGS=", "GOPROXY=off")
	if out, err := build.CombinedOutput(); err != nil {
		res.status = "WARN"
		res.detail = fmt.Sprintf("the probe did not compile: %v (%s)", err, firstLine(out))
		return res
	}

	out, err := exec.CommandContext(ctx, binary).CombinedOutput()
	if err != nil {
		res.status = "WARN"
		res.detail = fmt.Sprintf("compiled, but WOULD NOT RUN: %v (%s)", err, firstLine(out))
		return res
	}
	if strings.TrimSpace(string(out)) != token {
		res.status = "WARN"
		res.detail = "ran, but printed something unexpected: " + firstLine(out)
		return res
	}

	res.status = "OK"
	res.detail = "compiled and ran"
	return res
}

// unknownMakefileTools returns every tool a Makefile recipe invokes that the
// manifest above does not cover.
//
// This is the anti-drift binding. `TestDevcontainerInstallsEveryToolTheMakefileInvokes`
// (apps/api/internal/db/wiring_test.go) asks a DIFFERENT question of the same
// text -- does postCreate.sh INSTALL it -- so the two are not duplicates of one
// rule: that one guards a fresh container, this one guards the machine in front
// of you. What they share is the reading of a recipe, including the
// continuation-joining below, which that test's own comment explains at length.
func unknownMakefileTools(root string) []string {
	content, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		return nil
	}

	// Shell keywords and coreutils: not tools anyone installs.
	shell := map[string]bool{
		"cd": true, "rm": true, "mkdir": true, "echo": true, "if": true, "fi": true,
		"then": true, "else": true, "exit": true, "make": true, "for": true,
		"done": true, "set": true, "true": true, "false": true, "test": true,
	}
	known := map[string]bool{}
	for _, c := range manifest {
		known[c.probe[0]] = true
	}

	seen := map[string]bool{}
	var unknown []string
	for _, line := range recipeLines(string(content)) {
		for _, command := range strings.Split(strings.TrimPrefix(strings.TrimSpace(line), "@"), "&&") {
			fields := strings.Fields(strings.TrimSpace(command))
			// `GOTMPDIR=... go test` puts env assignments before the command.
			for len(fields) > 0 && strings.Contains(fields[0], "=") {
				fields = fields[1:]
			}
			if len(fields) == 0 {
				continue
			}
			name := fields[0]
			if strings.HasPrefix(name, "$") || strings.HasPrefix(name, "\"") ||
				shell[name] || known[name] || seen[name] {
				continue
			}
			seen[name] = true
			unknown = append(unknown, name)
		}
	}
	return unknown
}

// recipeLines returns each recipe as ONE logical line, joining backslash
// continuations. A continued `docker run ... \` recipe read physically becomes
// three commands named `docker`, `-v` and `golang:1.27`.
func recipeLines(makefile string) []string {
	var out []string
	var pending strings.Builder

	for _, line := range strings.Split(makefile, "\n") {
		if !strings.HasPrefix(line, "\t") && pending.Len() == 0 {
			continue
		}
		trimmed := strings.TrimRight(line, " \t\r")
		if strings.HasSuffix(trimmed, "\\") {
			pending.WriteString(strings.TrimSuffix(trimmed, "\\"))
			pending.WriteString(" ")
			continue
		}
		if pending.Len() > 0 {
			pending.WriteString(trimmed)
			out = append(out, pending.String())
			pending.Reset()
			continue
		}
		out = append(out, trimmed)
	}
	if pending.Len() > 0 {
		out = append(out, pending.String())
	}
	return out
}

// repoRoot walks up until it finds the marker only the repository root has. The
// Makefile invokes this from apps/api, so nothing can be relative to the caller.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "PROJECT_STATE.md")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("repository root not found: no PROJECT_STATE.md in any parent")
		}
		dir = parent
	}
}

var whitespaceRe = regexp.MustCompile(`\s+`)

func firstLine(out []byte) string {
	text := strings.TrimSpace(string(out))
	if text == "" {
		return "(no output)"
	}
	if index := strings.IndexAny(text, "\r\n"); index >= 0 {
		text = text[:index]
	}
	text = strings.TrimSpace(whitespaceRe.ReplaceAllString(text, " "))
	if runes := []rune(text); len(runes) > 90 {
		return strings.TrimRight(string(runes[:89]), " ,.;:") + "…"
	}
	return text
}
