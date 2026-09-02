# Verify Report: Phase 01 — Domain and Data

**Change**: `phase-01-domain-and-data` | **Mode**: full artifacts (proposal + design + tasks + specs; no apply-progress artifact — tasks were implemented task-by-task under direct user direction, recorded in per-task close-out notes in `tasks.md`) | **Verdict**: **PASS**

## Test execution (real, not inferred)

- `make test-api-container` (the only valid path on this Windows host — Windows Smart App Control blocks freshly linked Go test binaries, confirmed by the project's own T-01-020 diagnosis) → **exit 0**, all packages green:
  - `internal/config` — 0.004s
  - `internal/db` — 13.334s
  - `internal/db/dbtest` — 2.779s
  - `internal/db/rlstest` — 6.175s
  - `internal/httpapi` — 0.107s
  - `cmd/api`, `internal/api`, `internal/db/sqlcgen` — no test files
- `make lint-api`:
  - `golangci-lint run ./...` → 0 issues
  - `govulncheck ./...` → 0 called vulnerabilities (1 module-level vuln with no fix, not reachable — consistent with tasks.md's own recorded finding)
- `go.mod` dependency versions verified against tasks.md claims: `pgx/v5 v5.10.0`, `goose/v3 v3.27.3`, `testcontainers-go v0.44.0`, `google/uuid v1.6.0` — all match exactly.

## Structural verification

- 12 goose migrations present (`00001`…`00012`), matching design's migration plan exactly.
- `internal/db/rlstest/catalog.go`'s `Schema` declares 15 tenant tables, 9 `TenantChildren`, 4 `NonTenantModel`, 1 infrastructure exemption, 2 `AppendOnly`; `Pending` map is **empty** — confirms the "19 of 19 declared model tables verified, 0 pending" claim from source, not from a report.
- `internal/db/rlstest/isolation_test.go`'s `abCases()` returns exactly **15** `dbtest.TenantTable` literals (shelters, memberships, media, pets, pet_media, pet_health_records, pet_status_history, form_templates, form_template_versions, form_submissions, adoption_applications, application_events, application_notes, documents, audit_log). `TestTenantIsolation_CoversEveryDeclaredTenantTable` enforces this by enumeration in both directions, closing the "hand-compared list" failure mode the phase's own T-01-035 note warns about.
- `internal/db/tenant.go`'s `WithTenant`/`WithPublic` match the design contract verbatim: `Beginner`/`TxBeginner` interfaces (not the concrete pool), `set_config(..., true)` strictly inside Begin/Commit, rollback + re-panic on panic, rollback on error, no `SET LOCAL` anywhere.
- `internal/db/dbtest/roles.go`'s `CheckRoleCannotBypassRLS`/`guardRole` implement the `rolsuper`/`rolbypassrls`/owner-role guard exactly as the tenant-isolation spec requires, and the capabilities are returned (observable), matching T-01-008's own recorded gap-and-fix (a guard nobody observes is not a guard).
- `internal/db/query/*.sql` (9 files) + committed `internal/db/sqlcgen/` (8 generated files) present.

## The two flagged items — both confirmed

1. **Three Phase-07-moved scenarios are absent from the delta — confirmed.** `specs/dynamic-forms-data/spec.md` contains no "Duplicate field identifiers are rejected", "An unknown field type is rejected", or "An out-of-range span is rejected" scenario. In their place, an explicit blockquote at the *Field identifiers are stable* requirement records the move and its reasoning (`WHEN it is validated` = application-layer, this phase delivers no validator), and states what stayed (the field-removal-survives-versioning half, which the schema does enforce and T-01-024 asserts end to end). Independently confirmed present verbatim in `docs/vault/30-fases/FASE-07.md` under "Alcance heredado de Fase 01".

2. **Both T-01-026 spec amendments are present in `specs/dynamic-forms-data/spec.md` — confirmed.**
   - *Publishing creates a new version*: `UNIQUE (shelter_id, template_id, version)`, with an inline blockquote explaining why (PG 17: the unique index is checked before the FK, so a global key is a cross-tenant existence oracle even with the composite FK present). Migration `00007` matches.
   - *Templates, versions and submissions are tenant-scoped*: `form_submissions` SHALL reference its version by `(template_version_id, shelter_id)`, with a blockquote noting this closes T-01-025's mutation-round gap (mutant M2). Migration `00008` and `TestFormSubmissions_CannotReferenceAnotherTenantsVersion` / `TestTenantChildren_ReferenceTheirParentCompositely` implement and assert it.

## Tasks

All 35 tasks `[x]`. No apply-progress artifact exists — expected, given direct task-by-task execution. Each task carries a detailed close-out note in `tasks.md`: RED evidence, mutation-testing rounds with kill counts, accepted survivors with stated reasons, and deferred findings routed to specific later phases. This is a materially stronger evidentiary trail than a standard apply-progress table.

## Adversarial check performed

Spot-checked `append_only_test.go` for the ghost-loop/vacuous-pass failure mode strict-TDD-verify specifically flags: the enumerated suite carries a `verified == 0` guard and an explicit anti-vacuity row-count assertion before probing refusals — not a loop that could silently iterate zero times. No tautological or vacuous assertions found in the files sampled (`tenant.go`, `roles.go`, `catalog.go`, `append_only_test.go`, `isolation_test.go` header).

## Issues

None CRITICAL. None WARNING blocking archive. tasks.md itself already records several open, explicitly-deferred, non-blocking items (LT-2 shelter-verification not yet enforced, `refresh_tokens` access path, `app_public` on `shelters`, B1 privilege escalation, `pets.breed_id` composite FK, assignment to a non-active member, retention-purge semantics) — all carried forward with a named destination phase (Phase 02/06/08), not silently dropped, and all outside this phase's spec scope.

## Verdict

**PASS.** Recommend `sdd-archive`.
