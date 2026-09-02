# Archive Report: phase-01-domain-and-data

**Change**: `phase-01-domain-and-data`  
**Archived to**: `openspec/changes/archive/2026-09-01-phase-01-domain-and-data/`  
**Artifact Store**: hybrid (openspec + engram mirror)  
**Date Archived**: 2026-09-01  

---

## Final State Summary

**Status**: COMPLETE  
**Verification**: PASS (0 CRITICAL, 0 WARNING, 0 SUGGESTION)  
**Tasks**: 35 of 35 complete (`[x]`)  
**Specs Merged**: 7 delta specs → `openspec/specs/`  
**Archive Contents**: All artifacts preserved with full traceability  

---

## Artifact Traceability

All change artifacts retrieved from engram and verified against openspec files:

| Artifact | Observation ID | Source | Status |
|----------|----------------|--------|--------|
| Proposal | #37 | `openspec/changes/phase-01-domain-and-data/proposal.md` | ✓ Archived |
| Spec | #40 | `openspec/changes/phase-01-domain-and-data/specs/*/spec.md` | ✓ Merged to `openspec/specs/` |
| Design | #39 | `openspec/changes/phase-01-domain-and-data/design.md` | ✓ Archived |
| Tasks | #41 | `openspec/changes/phase-01-domain-and-data/tasks.md` | ✓ Archived |
| Verify-Report | #148 | Verification run 2026-09-01 22:15:15 | ✓ Verified PASS |

---

## Specifications Merged

All 7 delta specs have been copied mechanically to `openspec/specs/`. Since `openspec/specs/` was empty at the start of this change (Phase 01 is the first SDD change), each delta spec became the authoritative main spec:

1. **tenant-isolation** — RLS policy set, isolation guarantees, role model (15 tenant tables + 4 non-tenant + 1 infrastructure)
2. **schema-migrations** — 12 goose migrations (00001–00012), idempotent, reversible
3. **data-model-core** — Shelters, users, memberships, media, refresh_tokens; case-insensitive email via citext
4. **pet-catalog-data** — Pets, reference data (species/breeds), public catalog read-only
5. **dynamic-forms-data** — Published version immutability, field identifier stability, tenant scope (with verified amendments)
6. **adoption-flow-data** — Applications and child rows, closed status set, documents
7. **append-only-audit** — Append-only enforcement via triggers, three independent layers

**Merge Quality Assurance**:
- ✓ All 7 copied files verified with `diff -r` (no differences)
- ✓ `dynamic-forms-data` verified for two critical amendments (T-01-026 composite reference, version uniqueness key with cross-tenant oracle note)
- ✓ `dynamic-forms-data` verified for three Phase-07-moved scenarios (mentioned in context via blockquote, not present as active requirements)

---

## Verification Authority

Per `verify-report` (#148, 2026-09-01 22:15:15):

### Test Evidence (Real Execution, Not Inferred)
- `make test-api-container`: **exit 0** across all packages
  - `internal/config` 0.004s
  - `internal/db` 13.334s (suite carries 19-table A/B isolation + meta-test + append-only + public)
  - `internal/db/dbtest` 2.779s
  - `internal/db/rlstest` 6.175s (isolation, catalog, appendonly, public suites)
  - `internal/httpapi` 0.107s
- `golangci-lint run ./...`: **0 issues**
- `govulncheck ./...`: **0 called vulnerabilities**

### Structural Verification
- 12 goose migrations present (00001–00012), matching design plan exactly
- `internal/db/rlstest/catalog.go`: `Pending` map **empty** — confirms all 19 declared model tables verified
- `internal/db/rlstest/isolation_test.go`: `abCases()` returns exactly **15 tenant table cases** (one per tenant table)
- `WithTenant`/`WithPublic` match design contract: `set_config(..., true)` strictly inside Begin/Commit, no `SET LOCAL`, rollback+re-panic on error/panic
- Role guard implemented: `rolsuper = false AND rolbypassrls = false` for both `app_tenant` and `app_public`
- Composite FKs on pet children verified: cross-tenant attach → SQLSTATE `23503`

### Verdict
**PASS** — Recommend archive. No CRITICAL issues. No WARNING issues blocking archive.

---

## Implementation Record

**Mode**: Full artifacts (proposal + design + tasks + specs); no apply-progress artifact  
**Execution Method**: Task-by-task direct user execution (not `sdd-apply`)  
**Why No apply-progress**: Per launch prompt, all 35 tasks were implemented directly under user direction, with detailed close-out notes recorded per-task in `tasks.md`. This is a materially stronger evidentiary trail than a standard apply-progress table.

**Task Completion**:
- All 35 tasks: `[x]` (both in `openspec/changes/phase-01-domain-and-data/tasks.md` and `docs/vault/30-fases/FASE-01.md`)
- No stale unchecked tasks

**Deliverables Shipped**:
- 12 goose migrations (files 00001–00012)
- 5 ADRs written (ADR-0007, ADR-0008, ADR-0009, ADR-0010, ADR-0011) in `docs/vault/20-arquitectura/`
- RLS policies, role model, tenant isolation proofs
- `WithTenant`/`WithPublic` transaction wrappers
- 15 A/B tenant-isolation test cases + meta-test + orphan tests
- `app_public` read-only catalog suite
- Append-only enforcement (triggers + policy gaps)

---

## Open Questions & Deferred Findings

Seven questions were open at task close. **Two were decided by user on 2026-09-01; five were carried to later phases:**

| Question | Decision/Route | Destination | Evidence |
|----------|----------------|-------------|----------|
| Shelter verification (LT-2) | Deferred | Phase 06 | `docs/vault/30-fases/FASE-06.md` |
| `refresh_tokens` access path | Deferred | Phase 02 | `docs/vault/30-fases/FASE-02.md` |
| `app_public` policy on `shelters` | Deferred | Phase 06 | `docs/vault/30-fases/FASE-06.md` |
| Retention purge semantics | Deferred | Phase 08 | `docs/vault/30-fases/FASE-08.md` — "deletes `form_submissions`, keeps application row" |
| `pets.breed_id` composite FK | Deferred | Phase 06 | (recorded in tasks.md) |
| Assignment to non-active member | Deferred | Phase 06 | (recorded in tasks.md) |
| `pet_status_history` append-only | **Decided**: Ships as ordinary tenant table | This Phase | T-01-020 (no state change) |
| `CITEXT` vs `lower(email)` | **Decided**: Use `CITEXT` (Neon-available, verified) | This Phase | D7 reversal (2026-08-29) |

No open questions remain at archive close.

---

## Mutation Testing Summary

Twelve mutation rounds performed across implementation:

| Round | Context | Result | Survivors Accepted |
|-------|---------|--------|-------------------|
| M1 | Initial policy template | Kill count N/A | Template finalized |
| M2–M12 | Per-task refinements | Per-task records | All refined, 0 lingering |

**Learned**: No mutation round was left with an accepted survivor that could be re-opened. Each discovery led to a code change; no change is waiting for a decision.

---

## Engram Queue

- **Saved observations**: 52 (all with bidirectional traceability — `observation_id` written back into each artifact)
- **Discarded**: 2 (superseded by later versions)
- **Queue at close**: Empty (0 pending)

---

## Native Review Authority

`reviewGate` structurally absent. No review was started for this candidate; delivery proceeds under ordinary repository policy. Receipt-driven development was not enabled for this change.

---

## Archive Integrity

✓ Change folder moved from `openspec/changes/phase-01-domain-and-data/` to `openspec/changes/archive/2026-09-01-phase-01-domain-and-data/`  
✓ Source directory removed after move  
✓ Diff verification: no differences (empty diff output confirms byte-identical move)  
✓ All 7 delta specs copied to `openspec/specs/` with verified diff  
✓ Archive report written and added to archived folder  

---

## SDD Cycle Complete

This change has been fully planned (proposal), specified (7 delta specs), designed (with 9 decisions), tasked (35 ordered tasks), implemented (task-by-task), verified (PASS), and archived. The cycle is closed.

**Next recommended**: Next SDD change or `sdd-new` workflow.

---

**Archived by**: sdd-archive executor  
**Archive timestamp**: 2026-09-01 (execution date)  
**Artifacts in archive**:
- proposal.md ✓
- specs/ (7 capability specs) ✓
- design.md ✓
- tasks.md ✓
- archive-report.md (this file) ✓
