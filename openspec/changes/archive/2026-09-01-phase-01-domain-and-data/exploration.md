# Exploration — Phase 01: Domain and Data

> Implements **FASE-01** of the master plan (`docs/vault/30-fases/FASE-01.md`).
> Scope authority: master plan §3 (multi-tenancy) and §4 (data model), plus ADR-0002.
> Mirror of Engram topic `sdd/phase-01-domain-and-data/explore`.

## Current state

`apps/api/` is greenfield for this phase. Verified in `go.mod`: module
`github.com/gentleman/mascotapp/apps/api`, `go 1.27.0`, and the only dependency is
`github.com/go-chi/chi/v5 v5.3.2`. No pgx, sqlc, goose or testcontainers. There is no
`internal/domain`, `internal/db`, `internal/store` or `migrations/`.

Established conventions to follow:

- External `_test` packages, plain stdlib assertions (no testify), `t.Helper()`,
  selective `t.Parallel()`.
- A doc comment on every exported symbol that justifies the design choice rather than
  restating the name.
- Errors wrapped as `fmt.Errorf("component: %w", err)`.
- Dependencies injected through constructors.
- The `Makefile` owns every deletion; agents never call `rm` for build artifacts.

`docker-compose.yml` provides `postgres:17-alpine` and `minio`. There is no test
database, no roles and no schema yet. `.devcontainer/postCreate.sh` installs the
`goose` and `oapi-codegen` CLIs and provides the C compiler that the Windows host lacks.

`openspec/config.yaml` already pins `apply.tdd: true` and
`verify.coverage_threshold: 75`. `api/openapi.yaml` exposes only `/healthz` and
`/readyz`, consistent with Phase 01 being a data-layer-only change.

## Affected areas

| Path | Change |
|---|---|
| `apps/api/go.mod` | add pgx/v5, goose/v3, testcontainers-go (+ postgres module), a uuid v7 library |
| `apps/api/internal/db/migrations/` | new — goose SQL migrations: schema, roles, RLS |
| `apps/api/internal/db/` | new — sqlc-generated code plus the tenant transaction wrapper |
| `sqlc.yaml` | new — targets pgx/v5 |
| `Makefile` | new `migrate` target wrapping `goose up` |
| `docker-compose.yml`, `openspec/config.yaml` | unaffected |

## Approaches considered

1. **RLS tests via testcontainers-go.** Isolated and reproducible; identical on the
   Windows host and on Linux CI. testcontainers speaks the Docker API over the socket
   and is pure Go, so `CGO_ENABLED=0` does not affect it — cgo is only what `-race`
   needs. Cost: requires a reachable Docker daemon, plus container startup latency
   unless the container is reused per package.
2. **RLS tests against the docker-compose Postgres.** No new tooling, but shared
   mutable state, no isolation between parallel tests, and it conflates the development
   database with the test database. Cheap now, expensive later.
3. **goose CLI only.** Rejected: it leaves testcontainers databases empty, with nothing
   to bootstrap the schema before a test runs.
4. **goose embedded `fs.FS` Provider (v3.16+) plus the CLI.** `//go:embed migrations/*.sql`
   with `goose.NewProvider(dialect, db, fsys)` lets one migration set serve both the
   human `goose up` path and programmatic test bootstrap.

**Recommendation: 1 + 4 together.** One migration set stands up both the developer's
compose database and every test's ephemeral container, and RLS tests run the same way
locally and in CI without involving cgo.

## Role and connection topology

Two non-superuser roles:

- `app_tenant` — RLS active, read/write. The application's runtime pool.
- `app_public` — RLS active, read-only. The public catalog. Created in Phase 01 even
  though Phase 06 is its first consumer, so that every `shelter_id` table can satisfy
  the isolation-test rule at the moment it is created.

Neither is the compose `mascotapp` role, which is the migration and owner role. The
application must never connect as the owner: RLS does not apply to superusers, and that
is the exact failure ADR-0002 exists to prevent.

`pgxpool` reuses physical connections across requests, so the tenant setting must be
issued strictly inside `Begin`…`Commit`, never on a bare pooled connection, or the
scope leaks to whoever receives that connection next. Hence a mandatory wrapper:

```go
WithTenant(ctx, pool, shelterID, func(tx pgx.Tx) error { ... })
```

sqlc-generated `Queries` stay tenant-unaware and are bound to the transaction the
wrapper hands out, via `queries.New(tx)`.

### Correction to the plan's illustrative snippet

The master plan and ADR-0002 write the tenant setting as:

```sql
SET LOCAL app.shelter_id = '<uuid>'
```

`SET LOCAL` is a utility statement and **cannot take bind parameters**, so that form
forces string interpolation of the tenant id into SQL. Use the function form instead,
which does accept parameters:

```sql
SELECT set_config('app.shelter_id', $1, true)
```

The third argument `true` means "local to the current transaction", matching
`SET LOCAL` semantics. The design phase MUST carry the parameterised form forward
rather than copying the illustrative one.

## Ordering risk

- `shelters` → `users` → `memberships` first; everything else has a foreign key to
  `shelters(id)`.
- Both roles and their `GRANT`s must exist **before** any table enables RLS. Turning on
  `FORCE ROW LEVEL SECURITY` with no working policy for a role locks that role out.
- `media` lands early — `pets`, `pet_media` and `documents` reference it — even though
  upload wiring belongs to Phase 04.
- `form_templates` → `form_template_versions` → `form_submissions` in strict
  foreign-key order. Immutability (permanent `field.id`, publish means `version + 1`)
  must be right in the first migration; retrofitting it after real submissions exist
  means reconciling data written under weaker rules.
- `application_events` and `audit_log` get `REVOKE UPDATE, DELETE` in their first
  migration, not later.
- The uuid v7 generation strategy (Go-side versus a Postgres extension) must be decided
  before the first `CREATE TABLE`.

## Open questions for the proposal phase

1. Should `CREATE ROLE app_tenant` / `app_public` live inside an idempotent goose
   migration, or in a separate one-time provisioning step? `CREATE ROLE` is
   instance-wide rather than schema-scoped, which does not match goose's per-database
   replay model. The exploration leans toward an idempotent migration, for parity
   between testcontainers, compose and Neon — but it changes `goose down` semantics and
   needs an explicit decision.
2. Should sqlc-generated code be committed with a CI diff check, mirroring the existing
   `oapi-codegen` / `make generate` precedent? The plan does not say. Recommendation:
   mirror it explicitly rather than leaving it implicit.

## TDD sequence for RLS

Strict TDD is active, but RED is coarser here than usual: a policy cannot be
unit-tested without a database.

1. Write the integration test first against a testcontainers Postgres, migrated up to
   just before the new table. It fails because the schema and policy do not exist yet.
2. Write the migration: table, `ENABLE` and `FORCE ROW LEVEL SECURITY`, and the policy.
3. Regenerate sqlc.
4. The same test now runs the real A/B scenario — insert as tenant A through
   `WithTenant`, assert that tenant B's read, write and delete affect zero rows, and
   assert that tenant A still sees its own row. GREEN.

Do not force a finer loop than table-plus-policy together. A half-created table with no
policy is not a meaningful assertion target.

## Verified versus assumed

Recorded explicitly so that later phases do not inherit an assumption as a fact.

| Claim | Status |
|---|---|
| `go.mod` declares only chi | **verified** |
| compose runs `postgres:17-alpine` | **verified** |
| `openspec/config.yaml` pins `tdd: true` and coverage 75 | **verified** |
| Docker Desktop is installed (v29.7.2) | **verified** |
| The Docker daemon is reachable | **verified false at exploration time** — Docker Desktop was installed but not running |
| testcontainers-go is unaffected by `CGO_ENABLED=0` | **assumed, plausible** — must be proven empirically before `sdd-apply` depends on it |
| `SET LOCAL` cannot take bind parameters | **verified** against PostgreSQL semantics |
