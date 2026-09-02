//go:build ignore

// Command engram-index generates the vault index of Engram memory candidates.
//
// It reads the frontmatter of every file in `.engram/queue/` and writes
// `docs/vault/20-arquitectura/indice-engram.md`.
//
// Why this exists: Engram is a semantic index that only Claude Code can reach.
// Any other agent (Kiro, OpenCode, a cold session without MCP) sees the
// repository and nothing else. This projects the memory layer into the vault so
// the repository stays the operating truth, per rule IA-4 of the master plan.
//
// Why Go and not a scripting language: every tool a Makefile recipe invokes has
// to exist in a fresh devcontainer, and `TestDevcontainerInstallsEveryToolTheMakefileInvokes`
// enforces exactly that. `go` is already there; a Python interpreter is not.
//
// Run with `make engram-index`. Regenerating is idempotent: the output is a
// pure function of the queue, so a diff means the queue actually changed.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const summaryLimit = 130

var (
	frontmatterRe = regexp.MustCompile(`(?s)\A---\r?\n(.*?)\r?\n---\r?\n(.*)\z`)
	scalarRe      = regexp.MustCompile(`^([a-z_]+):\s*(.*)$`)
	whitespaceRe  = regexp.MustCompile(`\s+`)
)

type entry struct {
	fields  map[string]string
	file    string
	summary string
}

func (e entry) get(key string) string {
	if value, ok := e.fields[key]; ok && value != "" {
		return value
	}
	return "—"
}

func main() {
	root, err := repoRoot()
	if err != nil {
		fail(err)
	}

	queueDir := filepath.Join(root, ".engram", "queue")
	paths, err := filepath.Glob(filepath.Join(queueDir, "*.md"))
	if err != nil {
		fail(err)
	}
	if len(paths) == 0 {
		fail(fmt.Errorf("no queue files found under %s", queueDir))
	}
	sort.Strings(paths)

	entries := make([]entry, 0, len(paths))
	for _, path := range paths {
		parsed, ok, err := parse(path)
		if err != nil {
			fail(err)
		}
		if !ok {
			fmt.Fprintf(os.Stderr, "warning: no frontmatter in %s\n", filepath.Base(path))
			continue
		}
		entries = append(entries, parsed)
	}

	output := filepath.Join(root, "docs", "vault", "20-arquitectura", "indice-engram.md")
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		fail(err)
	}
	if err := os.WriteFile(output, []byte(render(entries)), 0o644); err != nil {
		fail(err)
	}

	relative, err := filepath.Rel(root, output)
	if err != nil {
		relative = output
	}
	fmt.Printf("wrote %s (%d entries)\n", filepath.ToSlash(relative), len(entries))
}

// repoRoot walks up from the working directory until it finds the marker that
// only the repository root has. The Makefile may invoke this from any
// directory, so the paths cannot be relative to the caller.
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

func parse(path string) (entry, bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return entry{}, false, err
	}

	match := frontmatterRe.FindSubmatch(content)
	if match == nil {
		return entry{}, false, nil
	}

	fields := map[string]string{}
	for _, line := range strings.Split(string(match[1]), "\n") {
		scalar := scalarRe.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if scalar != nil {
			fields[scalar[1]] = strings.Trim(strings.TrimSpace(scalar[2]), `"`)
		}
	}

	return entry{fields: fields, file: filepath.Base(path), summary: summarize(string(match[2]))}, true, nil
}

// summarize returns the first paragraph of the body, unwrapped and flattened to
// one table cell. Queue files are hard-wrapped markdown, so a single line is
// usually half a sentence.
func summarize(body string) string {
	var paragraph []string
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "---") ||
			strings.HasPrefix(line, "|") || strings.HasPrefix(line, ">") {
			continue
		}
		if line == "" {
			if len(paragraph) > 0 {
				break
			}
			continue
		}
		paragraph = append(paragraph, line)
	}

	if len(paragraph) == 0 {
		return "—"
	}

	text := strings.Join(paragraph, " ")
	text = strings.NewReplacer("**", "", "|", "/", "`", "").Replace(text)
	text = strings.TrimSpace(whitespaceRe.ReplaceAllString(text, " "))

	runes := []rune(text)
	if len(runes) <= summaryLimit {
		return text
	}
	return strings.TrimRight(string(runes[:summaryLimit-1]), " ,.;:") + "…"
}

func render(entries []entry) string {
	var saved, pending []entry
	for _, e := range entries {
		if e.fields["observation_id"] != "" {
			saved = append(saved, e)
		} else {
			pending = append(pending, e)
		}
	}

	var b strings.Builder
	fmt.Fprint(&b, "# Índice de memoria — Engram\n\n")
	fmt.Fprint(&b, "> **Generado por `make engram-index`. No editar a mano.**\n")
	fmt.Fprint(&b, "> Fuente: el frontmatter de `.engram/queue/*.md`.\n\n")
	fmt.Fprint(&b, "Engram es un índice semántico al que **solo llega Claude Code por MCP**.\n")
	fmt.Fprint(&b, "Cualquier otro agente —Kiro, OpenCode, una sesión fría sin MCP— ve el\n")
	fmt.Fprint(&b, "repositorio y nada más. Este archivo proyecta esa capa dentro del vault para\n")
	fmt.Fprint(&b, "que el repositorio siga siendo la verdad operativa (regla IA-4 del plan maestro).\n\n")
	fmt.Fprint(&b, "**El texto completo de cada decisión vive en su archivo de cola**, que está\n")
	fmt.Fprint(&b, "versionado. El `observation_id` es la misma nota dentro de Engram; sirve para\n")
	fmt.Fprint(&b, "trazabilidad, no es requisito para leerla.\n\n")
	fmt.Fprintf(&b, "- Candidatos totales: **%d**\n", len(entries))
	fmt.Fprintf(&b, "- Aprobados y guardados en Engram: **%d**\n", len(saved))
	fmt.Fprintf(&b, "- Pendientes de aprobación explícita del usuario: **%d**\n\n", len(pending))

	if len(pending) > 0 {
		fmt.Fprint(&b, "## Pendientes de aprobación\n\n")
		fmt.Fprint(&b, "Nada de esto está en Engram todavía. Requiere el sí explícito del usuario\n")
		fmt.Fprint(&b, "antes de `mem_save` (§6.6 del plan maestro).\n\n")
		fmt.Fprint(&b, "| Tarea | Tipo | Score | Archivo | Qué dice |\n")
		fmt.Fprint(&b, "|---|---|---|---|---|\n")

		sort.Slice(pending, func(i, j int) bool { return pending[i].file < pending[j].file })
		for _, e := range pending {
			fmt.Fprintf(&b, "| `%s` | %s | %s | [%s](../../../.engram/queue/%s) | %s |\n",
				e.get("task"), e.get("type"), e.get("score"), e.file, e.file, e.summary)
		}
		fmt.Fprint(&b, "\n")
	}

	fmt.Fprint(&b, "## Guardadas\n\n")

	// Every topic_key is unique, so grouping by the full key yields one section
	// per row. Group by its namespace instead — the taxonomy of the plan.
	byNamespace := map[string][]entry{}
	for _, e := range saved {
		topic := e.get("topic_key")
		namespace := topic
		if segments := strings.Split(topic, "/"); len(segments) > 2 {
			namespace = strings.Join(segments[:2], "/")
		}
		byNamespace[namespace] = append(byNamespace[namespace], e)
	}

	namespaces := make([]string, 0, len(byNamespace))
	for namespace := range byNamespace {
		namespaces = append(namespaces, namespace)
	}
	sort.Strings(namespaces)

	for _, namespace := range namespaces {
		group := byNamespace[namespace]
		sort.Slice(group, func(i, j int) bool {
			if group[i].get("task") != group[j].get("task") {
				return group[i].get("task") < group[j].get("task")
			}
			return group[i].file < group[j].file
		})

		fmt.Fprintf(&b, "### `%s/*` — %d\n\n", namespace, len(group))
		fmt.Fprint(&b, "| Tarea | `topic_key` | Score | Archivo | `observation_id` | Qué dice |\n")
		fmt.Fprint(&b, "|---|---|---|---|---|---|\n")
		for _, e := range group {
			segments := strings.Split(e.get("topic_key"), "/")
			leaf := segments[len(segments)-1]
			fmt.Fprintf(&b, "| `%s` | `…/%s` | %s | [%s](../../../.engram/queue/%s) | `%s` | %s |\n",
				e.get("task"), leaf, e.get("score"), e.file, e.file,
				e.fields["observation_id"], e.summary)
		}
		fmt.Fprint(&b, "\n")
	}

	return b.String()
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
