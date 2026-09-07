# Exploration — Phase 01, second pass (recovered from Engram)

> **Provenance.** This document was recovered on 2026-09-07 from the Engram
> observation `sdd/phase-01-domain-and-data/explore`, which had no counterpart
> on disk. It is a *different pass* from `exploration.md` in this same folder,
> not a copy: it carries three sections that file does not have —
> "Recommendation", "Risks" and "Ready for Proposal" — while `exploration.md`
> carries "Correction to the plan's illustrative snippet" and
> "Verified versus assumed", which this one lacks. Read both.
>
> Content below is verbatim, with CRLF normalised to LF.

---

## Exploration: Phase 01 — Domain and Data (migrations, RLS, sqlc, seeds, A/B tenant tests)

### Current State

`apps/api/` is greenfield for this phase. Verified in `go.mod`: module `github.com/gentleman/mascotapp/apps/api`, `go 1.27.0`, the ONLY dependency is `github.com/go-chi/chi/v5 v5.3.2` (confirmed in `go.sum` too — no pgx, sqlc, goose, or testcontainers anywhere yet). No `internal/domain`, `internal/db`, `internal/store`, or `migrations/` directories exist. `apps/api/internal/` currently has only `config/`, `httpapi/` (router, health, clientip), and `api/` (oapi-codegen output + its config.yaml).

Conventions observed (must be matched, not reinvented):
- External test packages (`package httpapi_test`), plain stdlib assertions, no testify.
- Table-driven where it fits, but simple handler tests are direct calls.
- `t.Helper()` on assertion helpers; `t.Parallel()` used selectively on tests that benefit from it (e.g. timing-sensitive readiness tests).
- Doc comments on every exported type/func explaining the *why*, not just the what (see `router.go`, `health.go`, `config.go` — each has a paragraph justifying a design choice).
- Errors wrapped with `fmt.Errorf("%w", ...)` and a `component: message` prefix style (`"config: invalid PORT %q..."`).
- Dependencies are injected as constructor args, never global state (`NewRouter(logger, clientIP)`).
- `Makefile` deletion policy: agents never call `rm` directly (denied by `.claude/settings.json`); all deletion lives in reviewed `Makefile` targets. Any migration/generated-code cleanup target must follow this.

`docker-compose.yml` provides `postgres:17-alpine` (user/db `mascotapp`, port 5432, `pg_isready` healthcheck) and `minio` as the R2 stand-in. This is the only Postgres available locally today — no test database, no additional roles, no schema.

`.devcontainer/postCreate.sh` installs `goose` and `oapi-codegen` as CLI tools via `go install`, and explicitly states the devcontainer has a C compiler for `-race` while the Windows host does not.

`openspec/config.yaml` already documents the intended stack for this phase (`sqlc + pgx/v5`, `goose`, "real-Postgres integration tests via Docker", "Mandatory per table with shelter_id: a tenant A / tenant B isolation test") and pins `apply.tdd: true` with `test_command: "cd apps/api && go test ./..."` (no `-race` locally, matching the Makefile comment). `verify.coverage_threshold: 75`.

`api/openapi.yaml` only documents `/healthz` and `/readyz` so far — no domain endpoints exist, consistent with Phase 01 being data-layer only (no HTTP surface).

### Affected Areas

- `apps/api/go.mod` — needs `pgx/v5`, `goose/v3`, `testcontainers-go` + its postgres module, and a uuid v7 library added.
- `apps/api/internal/db/migrations/` (new) — goose SQL migrations, source of truth for schema, roles, and RLS policies.
- `apps/api/internal/db/` or `internal/store/` (new) — sqlc-generated code + the mandatory tenant-transaction wrapper.
- `sqlc.yaml` (new, repo or `apps/api/` root) — sqlc config targeting `pgx/v5`.
- `docker-compose.yml` — unaffected for app-level tests (testcontainers spins up its own container), but stays as-is for `make dev` manual testing.
- `Makefile` — will need a `migrate`/`db-up` target wrapping `goose up`, gated by the plan's `ask` permission rule on `Bash(goose up*)`, and explicit `deny` on `goose down*`/`goose reset*`.
- `openspec/config.yaml` — already anticipates this; no changes needed for Phase 01 itself.

### Approaches

1. **RLS integration testing: testcontainers-go (postgres module)**
   - Pros: fully isolated per test run/package, no shared mutable state, identical code path on Windows dev host and Linux CI (pure Go, talks to the Docker daemon over its HTTP API — does **not** require cgo, unlike `-race`, which is the thing actually blocked by `CGO_ENABLED=0`/no-gcc on this host). Matches the project's own stated intent in `openspec/config.yaml` ("real-Postgres integration tests via Docker").
   - Cons: requires Docker Desktop running on the Windows dev host (not verified as running in this exploration — no shell/docker command was executed; this is a real open item, not an assumption). First run pulls `postgres:17-alpine`. Container startup adds latency per test package unless a session-scoped container is reused.
   - Effort: Medium (one shared test helper amortizes the cost across every `*_test.go` that needs RLS).

2. **RLS integration testing: point tests at the existing `docker-compose` Postgres**
   - Pros: zero new tooling, already running via `make dev`.
   - Cons: shared, mutable, developer-owned instance — tests must self-clean (truncate/rollback), parallel test runs collide, and it does not exist headless in CI unless compose is also stood up there (extra CI step, extra drift from local). Conflates "the DB a developer pokes at manually" with "the DB tests assert against," which is exactly the kind of accidental coupling the rest of this plan avoids elsewhere (e.g. `app_tenant`/`app_public` role separation).
   - Effort: Low to start, but hides cost that surfaces later as flaky/order-dependent tests.

3. **Migration execution: goose CLI only (developer runs `goose up` by hand)**
   - Pros: matches the devcontainer tooling already installed; simplest mental model for a human running `make dev`.
   - Cons: gives tests nothing to run against — a `testcontainers` container starts with an empty database, so something must apply migrations programmatically before the RLS tests can even attempt an insert. CLI-only leaves that gap unfilled.

4. **Migration execution: goose embedded `fs.FS`/`Provider` API + CLI, both**
   - Pros: `goose.NewProvider(dialect, db, fs)` (v3.16+) accepts any `fs.FS`, so `//go:embed migrations/*.sql` lets the same migration set run (a) via `goose up` CLI for a human against `docker-compose` Postgres, and (b) programmatically inside a `TestMain`/test helper against a fresh testcontainers instance — one source of truth, two callers. This is the standard pattern and avoids maintaining migrations in two places.
   - Cons: none material; slightly more setup than picking just one caller.
   - Effort: Low once the embed + provider wiring exists; it is one small package.

**Recommendation:** approach 1 (testcontainers-go) for RLS/tenant-isolation tests, approach 4 (goose embedded provider + CLI) for migrations. Together they let the exact same migration set stand up both the developer's `docker-compose` database and each test's ephemeral container, and they let RLS tests run identically on the Windows host (given Docker Desktop) and Linux CI (given Docker Engine) without ever touching `-race`/cgo. **Verification gap to close before `sdd-apply` writes any RLS test:** confirm Docker Desktop is actually reachable on this specific Windows host — this exploration did not execute `docker` or any shell command, so it is unverified, not assumed working.

### RLS role and connection topology (verified against ADR-0002 + the plan, not re-litigated — this is the "how")

- Two non-superuser Postgres roles are required: `app_tenant` (RLS-active, read/write — the pool the authenticated app connects with) and `app_public` (RLS-active, read-only — the pool the public catalog will use starting Phase 06, but the role and its policy should exist from Phase 01 so every table is "done" per the non-negotiable test requirement). Neither is the `mascotapp` role docker-compose provisions — that role acts as the migration/owner role, structurally analogous to what a superuser/admin connection would be in production, and **must never be the role the app runtime connects with** (ADR-0002's stated classic failure mode: RLS silently does nothing for a superuser or table owner without `FORCE ROW LEVEL SECURITY`).
- `SET LOCAL app.shelter_id = ...` must never touch a bare pooled connection: web research corroborates the ADR's own concern (`pgxpool` hands back a connection to the pool after every request, and the next request — very possibly a different tenant — reuses it; a session-level `SET` "sticks" to that physical connection and leaks scope across tenants). The setting has to be issued strictly *inside* `Begin`...`Commit`, which is exactly why ADR-0002 already commits to "se encapsula en un wrapper para que no sea opcional" (wrapped so it isn't optional). Concretely: a `WithTenant(ctx, pool, shelterID, fn func(pgx.Tx) error) error` helper that begins a tx, sets the GUC, runs `fn`, and commits/rolls back — no repository or handler is allowed to call the pool directly.
- Using `SELECT set_config('app.shelter_id', $1, true)` (parameterized) instead of string-formatting `SET LOCAL app.shelter_id = '<uuid>'` (as shown literally in the plan's illustrative snippet) avoids building SQL by hand for a value that, while it should always be a UUID from a validated JWT claim, doesn't need to be treated as a special case: Postgres's `SET` statement doesn't accept bind parameters, but `set_config()` is an ordinary parameterized function call. Worth calling out explicitly in the design phase since the plan's own example uses string interpolation.
- sqlc-generated code should stay a pure data-access layer (`Queries` bound to a `pgx.Tx` via `queries.New(tx)`), with zero knowledge of tenancy — the `WithTenant` wrapper is the only thing that ever calls `set_config`, and it hands the resulting tx to sqlc's generated `New(tx)`. This keeps the generated code swappable/regeneratable without re-encoding the security-critical wrapping logic inside it.

### Ordering risk

- `shelters` and `users`/`memberships` must land before anything referencing `shelter_id`, since every RLS policy and FK depends on `shelters(id)` existing.
- Both roles (`app_tenant`, `app_public`) and their `GRANT`s must exist before any table's RLS policy is enabled — `ENABLE`/`FORCE ROW LEVEL SECURITY` with no working policy for a role that needs access locks that role out entirely, which is a cheap mistake to make while iterating table-by-table.
- `media` is referenced by many tables (`pets.logo_media_id`/`cover_media_id`, `pet_media`, `documents`) — sequence it early even though Phase 04 is where uploads land, since Phase 01 only needs the schema+RLS, not the R2 wiring.
- `form_templates` → `form_template_versions` → `form_submissions` is a strict FK order, and the immutability rules (`field.id` immutable for life, publish creates `version + 1`, never edits in place) are schema-and-convention decisions that are expensive to reverse later: once real submissions reference a `template_version_id`, retrofitting immutability after the fact means reconciling data that was written under weaker rules. Get the `UNIQUE(template_id, version)` and the closed field-type union right in the first migration.
- `application_events` and `audit_log` must be append-only from their very first migration (`REVOKE UPDATE, DELETE` from `app_tenant` on those two tables, or an equivalent trigger) — not bolted on later, since any code written against them in the meantime could come to depend on mutating them.
- Deciding the **uuid v7 generation strategy** (Go-side, e.g. `google/uuid`'s `NewV7()`, vs. a Postgres extension) has to happen before the very first `CREATE TABLE`, since it determines every PK's default/insertion path project-wide. No such dependency exists in `go.mod` yet.

### Open Questions (genuinely need a decision, not answered by the plan already)

1. **Role provisioning location.** Should `CREATE ROLE app_tenant`/`app_public` live inside an idempotent goose migration (`DO $$ ... IF NOT EXISTS $$`), so that testcontainers-created databases and the real Neon instance are provisioned identically? Or should role creation be a separate one-time script outside the replayable up/down migration set, since `CREATE ROLE` is instance-wide rather than schema-scoped and goose migrations are conventionally expected to be safely replayable per-database? Recommendation leaning toward "inside a migration, made idempotent" — because RLS test parity with production is the entire point of this phase's non-negotiable test requirement — but this changes how `goose down` behaves and should be an explicit call in the proposal, not something `sdd-apply` decides mid-implementation.
2. **Generated sqlc code: committed or gitignored?** The plan doesn't say. The existing `oapi-codegen` precedent in this repo commits generated output and has CI fail on any diff after re-running `make generate`. Recommend mirroring that exact pattern for sqlc for consistency, but it should be stated explicitly in the proposal rather than assumed.

### TDD red-test-first sequence for RLS specifically (Strict TDD Mode is active)

A policy cannot be unit-tested without a live database, so the RED step for any RLS-bearing table is coarser than a typical Go unit test loop: (1) write the integration test first — it opens a testcontainers Postgres, migrates up to the point *before* the new table/policy exists, inserts nothing (or fails to compile against not-yet-generated sqlc queries) — this is RED because the schema/policy the test exercises doesn't exist yet, not because of a failing assertion on working code; (2) write the migration creating the table with `ENABLE`/`FORCE ROW LEVEL SECURITY` and the tenant policy; (3) regenerate sqlc; (4) the same test now runs a real A/B scenario: insert as tenant A (`WithTenant` → shelter A), assert tenant B's `SELECT`/`UPDATE`/`DELETE` on that row affects zero rows, assert tenant A can still read/write its own row — GREEN. `sdd-apply` should not try to force a finer-grained red/green loop than this; the schema-then-policy step is atomic in practice because a half-created table with no policy is not a meaningful intermediate state to assert against.

### Recommendation

Proceed to `sdd-propose` with: testcontainers-go for RLS/tenant-isolation integration tests, goose's embedded `fs.FS` Provider for migrations (shared between the CLI path and the test-bootstrap path), `sqlc` targeting `pgx/v5` with generated code committed (mirroring the `oapi-codegen` precedent), and a mandatory `WithTenant` transaction wrapper using `set_config()` as the only place `SET LOCAL`-equivalent tenant scoping happens. Before `sdd-apply` starts the first RLS test, verify Docker Desktop is actually reachable on this Windows host — that is unverified, not assumed.

### Risks

- Docker Desktop availability on the Windows dev host is unverified — if unavailable, RLS integration tests cannot run locally at all (only in CI/devcontainer), which would slow the strict-TDD loop for Phase 01 specifically.
- `CREATE ROLE` inside a goose migration is a slightly unusual pattern (instance-wide, not schema-scoped) and needs explicit sign-off in the proposal, not a default assumed by `sdd-apply`.
- The plan's own RLS policy snippet uses string-interpolated `SET LOCAL`; the design phase must correct this to a parameterized `set_config()` call so the proposal doesn't silently copy an unsafe pattern from the source document.
- Getting `form_template_versions` immutability or the append-only enforcement on `application_events`/`audit_log` wrong in the first migration is expensive to reverse once later phases start depending on the (wrong) behavior.

### Ready for Proposal

Yes. Scope, constraints, and testing strategy are concrete enough to write `proposal.md`. The two open questions above (role-provisioning location, sqlc commit policy) should be stated as explicit decisions in the proposal rather than left implicit.
