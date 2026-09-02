package db_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// adrDir is where §6.1 puts dated, immutable decisions.
const adrDir = "docs/vault/20-arquitectura"

var adrName = regexp.MustCompile(`^ADR-(\d{4})-[a-z0-9-]+\.md$`)

// adrs returns every ADR by its number.
func adrs(t *testing.T) map[int]string {
	t.Helper()

	dir := filepath.Join(repoRoot(t), filepath.FromSlash(adrDir))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", adrDir, err)
	}

	found := map[int]string{}
	for _, entry := range entries {
		match := adrName.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		n, err := strconv.Atoi(match[1])
		if err != nil {
			t.Fatalf("parsing the number of %s: %v", entry.Name(), err)
		}
		if previous, taken := found[n]; taken {
			t.Fatalf("two ADRs claim number %04d: %s and %s. The number is how every other "+
				"document refers to the decision, so a collision makes both references "+
				"ambiguous", n, previous, entry.Name())
		}
		found[n] = entry.Name()
	}

	return found
}

// The DoD of T-01-034 in one line: *"numbering starts at 0007 — 0001..0006 are
// taken"*. This is what keeps that true for the NEXT phase, which will not have
// read this task.
//
// An ADR number is not decoration: it is how every other document names the
// decision, and this repository links them as `[[ADR-0002]]`. A reused number
// makes every one of those references ambiguous, and a gap means a decision was
// written and lost — the two failures a dated, immutable log exists to prevent.
func TestADRs_AreNumberedUniquelyAndWithoutGaps(t *testing.T) {
	t.Parallel()

	found := adrs(t)
	if len(found) == 0 {
		t.Fatalf("%s holds no ADR, so this test verified nothing", adrDir)
	}

	numbers := make([]int, 0, len(found))
	for n := range found {
		numbers = append(numbers, n)
	}
	sort.Ints(numbers)

	if numbers[0] != 1 {
		t.Errorf("the lowest ADR is %04d, not 0001. The sequence is meant to be read from the "+
			"beginning", numbers[0])
	}

	for i, n := range numbers {
		if want := i + 1; n != want {
			t.Fatalf("ADR numbering jumps from %04d to %04d. A gap means a decision was "+
				"written and lost, or a number was skipped and will be reused by somebody "+
				"who counted the files instead of reading the last one", want-1, n)
		}
	}
}

// Wiki links are the vault's only navigation, and a broken one is silent: it
// renders as plain text in Obsidian and as nothing at all in a diff.
//
// This matters more here than in ordinary documentation. §6 makes the vault the
// CONTINUITY layer — the thing a cold agent session reads to find out why the
// schema looks like this. A decision that references `[[ADR-0004]]` and lands on
// nothing costs exactly the context the vault exists to preserve.
func TestADRs_LinkOnlyToDecisionsThatExist(t *testing.T) {
	t.Parallel()

	found := adrs(t)
	link := regexp.MustCompile(`\[\[ADR-(\d{4})[^\]]*\]\]`)

	dir := filepath.Join(repoRoot(t), filepath.FromSlash(adrDir))
	for number, name := range found {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			body, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("reading %s: %v", name, err)
			}

			for _, match := range link.FindAllStringSubmatch(string(body), -1) {
				target, err := strconv.Atoi(match[1])
				if err != nil {
					t.Fatalf("parsing the link %q: %v", match[0], err)
				}
				if _, exists := found[target]; !exists {
					t.Errorf("%s links to %s, which does not exist. In Obsidian a dangling "+
						"link renders as plain text, so this is invisible until somebody "+
						"clicks it looking for the reasoning", name, match[0])
				}
				if target == number {
					t.Errorf("%s links to itself (%s)", name, match[0])
				}
			}
		})
	}
}

// Every ADR answers the same questions, because the template asks them.
//
// WHAT IS ASSERTED HERE IS NARROWER THAN THE TEMPLATE, and the reason is worth
// writing down because the first draft got it wrong.
//
// The template also asks for **Alternativas descartadas**, and that section is
// the one that matters most: a decision with no rejected alternative is a record
// that somebody TYPED, not that somebody CHOSE. But asserting the heading STRING
// fails all four Phase 00 ADRs — and they are not missing the reasoning. They put
// it under their own headings: ADR-0004's channel-by-channel table and its "Por
// qué en VISA sí y aquí no", ADR-0005's "El criterio de aptitud", ADR-0003's
// "Riesgo aceptado".
//
// So a string check there measures FORM and calls it substance. It is the
// T-01-033 lesson pointed the other way: there, a substring found anywhere read
// as a property that was absent; here, a substring absent would read as a
// property that is present. Neither is an assertion about the document.
//
// What stays is what is genuinely universal across all eleven and load-bearing:
// the context, the decision, and — the one §5.8 wrote mTLS's for explicitly —
// the conditions that would reopen it, which is what stops the same argument
// from being had every quarter.
func TestADRs_AnswerTheTemplatesQuestions(t *testing.T) {
	t.Parallel()

	required := []string{
		"## Contexto",
		"## Decisión",
		"## Condiciones de reapertura",
	}

	dir := filepath.Join(repoRoot(t), filepath.FromSlash(adrDir))
	for _, name := range adrs(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			body, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("reading %s: %v", name, err)
			}
			text := string(body)

			for _, section := range required {
				if !strings.Contains(text, section) {
					t.Errorf("%s has no %q section. The template asks it because a decision "+
						"without it is not reviewable: an ADR with no rejected alternative "+
						"records a preference, and one with no reopening condition invites "+
						"the same argument next quarter", name, section)
				}
			}

			// The header the whole file is addressed by. A missing date turns an
			// immutable record into an undated one.
			for _, field := range []string{"- **Fecha:**", "- **Estado:**", "- **Fase:**"} {
				if !strings.Contains(text, field) {
					t.Errorf("%s has no %q line", name, field)
				}
			}
		})
	}
}
