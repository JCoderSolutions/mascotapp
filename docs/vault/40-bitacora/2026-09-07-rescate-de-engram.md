---
fecha: 2026-09-07
fase: "02"
agente: claude-opus-5
tipo: rescate
---

# Rescate de observaciones que solo vivian en Engram

Al depurar Engram (147 observaciones, 388.028 caracteres) se encontraron cuatro
que no tenian contraparte en disco. Tres de ellas se conservan aca antes de
borrarlas de Engram; la cuarta, la exploracion de la Fase 01, fue a su lugar
natural en openspec/changes/archive/2026-09-01-phase-01-domain-and-data/.

**Este archivo es un deposito, no documentacion vigente.** Los tres documentos
de abajo son instantaneas fechadas que ya no describen el estado del proyecto.
Se guardan para que el borrado de Engram no pierda nada, y pueden podarse
enteros cuando dejen de importar.

| Documento | Fecha original | Por que se rescato |
|---|---|---|
| Exploracion de Fase 02 | 2026-09-01 | Resumen condensado; el archivo exploration.md es mas rico |
| Progreso de T-02-004 | ~2026-09-02 | Instantanea de progreso; tasks.md lleva el registro vigente |
| Capacidades de testing | 2026-08-28 | Anterior a testcontainers y al harness de mutacion |

El contenido va literal, con CRLF normalizado a LF. Esta en ingles porque asi
fue generado.

---

## 1. Exploracion de Fase 02 (Engram: sdd/phase-02-auth-and-multitenancy/explore)

**What**: Exploration of Phase 02 (auth + multi-tenancy) against the actual Phase 01 codebase. No HTTP auth layer exists at all yet — apps/api/internal/httpapi only has health/readyz + chi router + ClientIPResolver. No JWT/Argon2/TOTP library in go.mod (only indirect golang.org/x/crypto). openapi.yaml has zero auth paths.

**Why**: Team lead requested investigation-only pass to report what code constrains before /sdd-propose, focused on the 3 inherited-from-Phase-01 open decisions.

**Where**:
- apps/api/internal/db/tenant.go — WithTenant/WithPublic, the only DB door (ADR-0008). set_config(...,true), transaction-local GUC app.shelter_id.
- apps/api/internal/db/migrations/00002_tenancy_identity.sql — users (nullable password_hash, citext email), memberships (UNIQUE user_id,shelter_id), refresh_tokens (default-deny: RLS on, NO policy, NO grant — deliberate, stricter than any policy).
- apps/api/internal/db/migrations/00009_adoption_applications.sql — applicant_visible_users permissive policy; documents the B1 exposure (INSERT on adoption_applications = mint READ on users PII) deferred to column-level grants in Phase 02/03.
- apps/api/internal/db/migrations/00001_extensions_roles_and_grants.sql + bootstrap.go — app_tenant/app_public roles created in SQL w/ NOBYPASSRLS; roleNamePattern `^app_[a-z][a-z0-9_]{0,40}$` already accepts a future app_auth role name without change.
- apps/api/internal/httpapi/clientip.go — rightmost-untrusted-IP resolver already built for future rate limiting; ClientIPFromContext exists.
- ADR-0008 (WithTenant is the only door), ADR-0009 (roles/grants/default-deny, explicitly lists app_auth or B1 column grants as its own reopening conditions), ADR-0007 (composite FK denormalization pattern — relevant precedent for the assignee/member-visibility gap).
- openspec/specs/tenant-isolation/spec.md — codifies refresh_tokens default-deny as a spec requirement already, and users/refresh_tokens/species/breeds as the "non-tenant model table" set.
- apps/api/internal/db/query_test.go — TestQueries_DoNotFilterByShelterID convention (query never filters by shelter_id, policy does); TestQueries_ExerciseEveryDeclaredTable exempts refresh_tokens via rlstest.Schema.NoPolicy — that exemption AUTO-EXPIRES the day Phase 02 gives it a grant, forcing a users.sql-style query to exist.
- docs/vault/10-propuesta/analisis-y-plan.md §5.2/§5.3 (lines ~323-332) — governing auth/authz requirements text.

**Learned**:
1. refresh_tokens access-path decision (app_auth role vs app.user_id GUC) is genuinely still open in code; nothing pre-commits either way except ADR-0009's naming convention already accommodating app_auth.
2. B1 fix (column-level grants) has a concrete regression trip-wire already in place: TestApplicantPolicy_IsAsWideAsWritingAnApplication (apps/api/internal/db/rlstest/adoption_applications_test.go) goes red the day grants land — it's currently pinning the wide/vulnerable state on purpose.
3. Assignee/member-visibility gap (assignable ⊋ readable) is pinned by TestAssignment_DoesNotYetRequireAnActiveMembership (rlstest/adoption_flow_test.go, scope_test.go) — same "goes red when fixed" pattern.
4. No email delivery infra wired yet (Resend mentioned only in plan prose, no client/adapter code) — magic link needs this built from scratch.
5. Plan's TOTP requirement is silent on which TOTP secret format/library and on recovery codes — real open question for propose phase.
6. .env.example read is blocked by this session's permission settings (directory denied) — could not confirm JWT_SECRET/RESEND_API_KEY presence; flagged rather than assumed.

---

## 2. Progreso de T-02-004 (Engram: sdd/phase-02-auth-and-multitenancy/apply-progress)

**What**: T-02-004 closed (PR-02-03, branch `feat/pr-02-03-auth-queries` on `feat/pr-02-02-auth-role-migration`). `refresh_tokens` removed from `rlstest.Schema.NoPolicy`; `apps/api/internal/db/query/auth.sql` added (`GetRefreshTokenByHash`, `RevokeRefreshTokenFamily`, `GetUserCredentialsByEmail`, `ListOwnMemberships`); new meta-test `TestPolicies_DoNotCrossGUCs` (P2-D1) in `apps/api/internal/db/rlstest/guc_test.go`, scans `pg_policy` via `pg_get_expr` and fails if an `app_tenant` policy mentions `app.user_id` or an `app_auth` policy mentions `app.shelter_id`.

**Why**: task contract in `openspec/changes/phase-02-auth-and-multitenancy/tasks.md` (T-02-004).

**Where**: `apps/api/internal/db/rlstest/catalog.go`, `apps/api/internal/db/rlstest/guc_test.go` (new), `apps/api/internal/db/query/auth.sql` (new), `apps/api/internal/db/sqlcgen/auth.sql.go` (generated), `apps/api/internal/db/sqlcgen/querier.go`, `apps/api/internal/db/migrate_roundtrip_test.go`.

**Learned**: removing `refresh_tokens` from `NoPolicy` broke a SECOND test the task text did not mention — `TestMigrations_EveryStepDownLeavesAConsistentSchema`. That stepwise-rollback walk calls `CheckProtection` at every INTERMEDIATE migration version, not just HEAD; between `00002` (table created, RLS on, no policy) and `00013` (first real policy), `refresh_tokens` legitimately has zero policies, and an unversioned `NoPolicy`/`CheckProtection` cannot express "exempt only before version 13." Fixed with `Classification.PolicyLandsAt map[string]int64` (mirrors `Pending`'s shape but for a different gap: `Pending` = table doesn't exist yet, `PolicyLandsAt` = table exists but its first policy lands at a later migration) plus `CheckProtectionAt(t, at)`, which the stepwise walk now calls instead of the unversioned `CheckProtection`. `CheckProtection` itself is unchanged in behavior — HEAD-only checks stay exactly as strict as before. Full memory writeup: `.engram/queue/2026-09-02-policylandsat-version-aware-catalog-exemption.md` (score 4, unreviewed).

Evidence: `make test-api-container` full suite green (`internal/db`, `internal/db/dbtest`, `internal/db/rlstest` all `ok`). Named `-run -v` confirmation: `TestQueries_ExerciseEveryDeclaredTable` PASS, `TestSchema_MatchesTheDeclaredCounts` PASS (4 non-tenant / 19 model, unchanged), `TestCatalog_EveryRelationIsClassifiedAndProtected` PASS ("verified 19 of 19 declared model tables; 0 still pending"), `TestMigrations_EveryStepDownLeavesAConsistentSchema` PASS ("walked 13 migration(s); inspected 150 model table(s)"), `TestPolicies_DoNotCrossGUCs` PASS, `TestPolicyRow_Mentions` PASS (4 sub-cases). `make generate` clean diff (only `sqlcgen/auth.sql.go` new + `querier.go`; oapi-codegen/web untouched). `golangci-lint run ./...`: 0 new issues (1 pre-existing gosec G101 in `dbtest/container.go`, not touched by this task).

---

**T-02-016 closed (PR-02-04, branch `feat/pr-02-04-totp-recovery-codes` on `feat/pr-02-03-auth-queries`, commit `a90a56f`).** Migration `00014_totp_recovery_codes.sql`: table (`id`, `user_id → users(id) ON DELETE CASCADE`, `code_hash bytea UNIQUE`, `used_at`, `created_at`), `ENABLE`+`FORCE` RLS and the `auth_own_recovery_codes` policy (`TO app_auth USING/WITH CHECK (user_id = app.user_id)`) in the SAME migration as the table — no `PolicyLandsAt` entry needed, unlike `refresh_tokens`. Grants: `SELECT, INSERT, DELETE` + `UPDATE (used_at)` only (P2-D8). `totp_recovery_codes` joins `rlstest.Schema.NonTenantModel`, moving `TestSchema_MatchesTheDeclaredCounts` 4→5 non-tenant / 19→20 model in the same commit as the table. `query/auth.sql` gained `InsertRecoveryCode`, `DeleteRecoveryCodesForUser`, `GetRecoveryCodeByHash`, `RedeemRecoveryCode`. New file `apps/api/internal/db/rlstest/totp_recovery_codes_test.go` with the two data-model-core integration scenarios.

**Where**: `apps/api/internal/db/migrations/00014_totp_recovery_codes.sql` (new), `apps/api/internal/db/rlstest/catalog.go`, `apps/api/internal/db/rlstest/catalog_test.go`, `apps/api/internal/db/rlstest/isolation_test.go`, `apps/api/internal/db/rlstest/totp_recovery_codes_test.go` (new), `apps/api/internal/db/query/auth.sql`, `apps/api/internal/db/sqlcgen/{auth.sql.go,models.go,querier.go}` (generated).

**Learned**: a THIRD pinned test the task text did not name — `TestTenancyPolicies_ApplyToTheRightRoleAndCommand` (`isolation_test.go`) — hardcodes the full cross-schema policy AND privilege inventory (`has_table_privilege` per role/table/privilege), and needed one policy row plus six privilege rows for `totp_recovery_codes`. The catalog meta-test only counts policies; it cannot see who they're for, so this second hand-written test is the only thing that would catch a policy pointed at the wrong role. The test's own doc comment already states the rule ("every migration from here adds its rows to both tables below") — `refresh_tokens` paid it in `00013`, this is the second payment. Full write-up: `.engram/queue/2026-09-02-totp-recovery-codes-third-pinned-inventory.md` (score 3, unreviewed).

RED confirmed against the real container BEFORE the migration existed: `TestCatalog_EveryRelationIsClassifiedAndProtected` failed with `"totp_recovery_codes" is declared and is not listed as pending, but does not exist`; both new scenario tests failed with `relation "totp_recovery_codes" does not exist` (SQLSTATE `42P01`).

Evidence: full `make test-api-container` green (`internal/db`, `internal/db/dbtest`, `internal/db/rlstest`, `internal/httpapi` all `ok`), including `TestMigrations_EveryStepDownLeavesAConsistentSchema` ("walked 14 migration(s); inspected 170 model table(s)") — confirming no `PolicyLandsAt` entry is needed for this table. Named `-run -v` confirmation: `TestCatalog_EveryRelationIsClassifiedAndProtected` PASS ("verified 20 of 20 declared model tables"), `TestSchema_MatchesTheDeclaredCounts` PASS, `TestTenancyPolicies_ApplyToTheRightRoleAndCommand` PASS, `TestTOTPRecoveryCodes_StoredHashedNotPlaintext` PASS, `TestTOTPRecoveryCodes_RedemptionIsSingleUse` PASS. `make generate` clean diff (`sqlcgen/auth.sql.go`, `models.go`, `querier.go` only). `golangci-lint run ./...`: 0 new issues (the one pre-existing gosec G101 in `dbtest/container.go`, untouched by this task).

Measured 280 authored lines against the 160 estimate — within `PR-02-04`'s share of the 400-line PR budget, no `size:exception` needed.

Progress: T-02-001 [x], T-02-002 [x], T-02-003 [x] (PR-02-02, `size:exception` accepted), T-02-004 [x] (PR-02-03), T-02-016 [x] (PR-02-04, this entry). Next: T-02-005 (PR-02-05, slice b — pins PostgreSQL column-privilege SQLSTATE semantics, self-contained, est 55).

---

## 3. Capacidades de testing (Engram: sdd/mascotapp/testing-capabilities)

**What**: Testing capability matrix for `mascotapp`, detected 2026-08-28.

`strict_tdd: true` — resolved from the explicit "Strict TDD Mode: enabled" marker in the global CLAUDE.md, not from a fallback.

| Capability | Tool | Status |
|---|---|---|
| Go toolchain | go 1.27.0 windows/amd64 | INSTALLED |
| Go unit/integration tests | `go test -race` | available (built in) |
| Go linter | `golangci-lint` | **NOT INSTALLED — gap for T-00-008** |
| Go vuln scan | `govulncheck` | not installed |
| Node runtime | node v22.23.1 / npm 10.9.8 | INSTALLED |
| Frontend unit tests | Vitest + Testing Library | planned, not installed |
| Type checker | `tsc --noEmit` | planned, not installed |
| E2E | Playwright | planned (Phase 11) |
| Containers | Docker 29.7.2 | INSTALLED (needed for testcontainers + local Postgres/MinIO) |
| CI | GitHub Actions | **no `.github/workflows/` yet — T-00-008** |

Planned test layers: domain unit tests (pure Go, table-driven, no DB) · repository integration tests against real Postgres via Docker · contract tests against `api/openapi.yaml` · Playwright E2E on the critical journey · **RLS tenant-isolation tests (mandatory per table with `shelter_id`)**.

Coverage gates: domain >= 90%, overall >= 75%.

**Why**: Strict TDD means every code task starts with a failing test. Knowing which runners actually exist prevents writing a Definition of Done that references tooling nobody installed.

**Where**: Makefile (`make test`, `make lint`) · docs/vault/30-fases/FASE-00.md (T-00-008)

**Learned**: `golangci-lint` and `govulncheck` are NOT installed despite being referenced in the plan's Definition of Done and in `make lint`. T-00-008 must install them before any DoD can honestly claim "lint clean". Docker being present is what makes real-Postgres integration tests viable, which the RLS isolation tests require — they cannot be faked with mocks.
