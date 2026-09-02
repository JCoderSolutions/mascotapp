# Proposal: Phase 01 — Domain and Data

> Implements **FASE-01** of the master plan. Authority: plan §3/§4 and ADR-0002.
> Input: `exploration.md` / Engram `sdd/phase-01-domain-and-data/explore`.

## Intent

`apps/api` has no schema, no roles, no persistence layer. Phase 01 makes shelter
isolation **verifiable rather than trusted**: the 19 tables of §4 (the plan's "~15"
undercounts), non-superuser roles, per-table RLS policies, and an A/B isolation test
per tenant table — the ADR-0002 completion rule.

## Scope

### In Scope

- goose migrations (embedded `fs.FS` + CLI), `app_tenant` / `app_public` roles, RLS.
- All 19 tables of §4, their indexes (§4.6), and `species`/`breeds` seeds.
- `pgxpool` + `WithTenant` transaction wrapper; `sqlc` config and generated queries.
- A/B isolation integration test per tenant-scoped table (testcontainers-go).

### Out of Scope

- HTTP handlers, JWT middleware, repositories above `internal/db` (Phase 02+).
- R2/media upload wiring (Phase 04); public catalog endpoints (Phase 06).
  `app_public` and `media` are created now so every table can satisfy the test rule.

## Capabilities

### New Capabilities

- `tenant-isolation`: roles, GUC contract, per-table policies, `WithTenant`, A/B rule.
- `schema-migrations`: goose conventions, ordering, idempotent provisioning, bootstrap.
- `data-model-core`: shelters, users, memberships, refresh_tokens, media.
- `pet-catalog-data`: species, breeds, pets, pet_media, health records, status history.
- `dynamic-forms-data`: templates, immutable published versions, submissions.
- `adoption-flow-data`: applications, notes, documents.
- `append-only-audit`: `application_events` + `audit_log`, UPDATE/DELETE revoked.

### Modified Capabilities

None — `openspec/specs/` is empty (verified).

## Approach

Migration `002` provisions roles **before** any table enables RLS. Every tenant table
lands as one unit: `CREATE TABLE` + `ENABLE`/`FORCE ROW LEVEL SECURITY` + policy +
grants, with its A/B test written first (RED) against a testcontainers Postgres.

Tenant scope is set **only** inside `Begin`…`Commit`, via the parameterised form:

```sql
SELECT set_config('app.shelter_id', $1, true)
```

`SET LOCAL` is forbidden — it is a utility statement and cannot take bind parameters.
sqlc `Queries` stay tenant-unaware, bound to the wrapper's `tx`.

## Decisions

| # | Decision | Rationale | Rejected |
|---|---|---|---|
| D1 | Roles created in an **idempotent goose migration** (`DO` block guarded on `pg_roles`), `NOSUPERUSER NOCREATEDB NOCREATEROLE`, **no password in SQL**; grants re-applied unconditionally | One code path across testcontainers / compose / Neon; RLS correctness depends on identical roles everywhere. `CREATE ROLE` is cluster-wide but the guard makes per-database replay safe | Separate provisioning step — two divergent paths, and the test container would need its own |
| D1b | `Down` **revokes grants, never drops roles** | A role may own objects or be in use by another database in the cluster; `DROP ROLE` would fail or cascade | Symmetric down — unsafe |
| D2 | sqlc output **committed**, `sqlc generate` added to `make generate`, CI fails on diff | Mirrors the existing `oapi-codegen` precedent (Makefile:29-34); `go build` needs no extra toolchain; the SQL is visible in review, which RLS review requires | Build-time generation — hides queries from review, adds a required CLI to every build |
| D3 | uuid v7 generated **in Go** (`uuid.NewV7()`); PK columns `uuid NOT NULL`, **no `DEFAULT`** | Removes any extension dependency. PG 17 (compose) has no native `uuidv7()`; ids known before INSERT, which event/audit correlation needs | `pg_uuidv7` — needs `CREATE EXTENSION` + a Neon allow-list we cannot confirm; `gen_random_uuid()` — v4, loses time ordering; native `uuidv7()` — PG 18 only, forks compose vs Neon |
| D4 | All 19 tables ship in Phase 01, split into **4 chained PRs**; nothing deferred to a later phase | See slices below | Cutting tables to fit 400 lines — would leave the schema half-built and unspecifiable |

### Deferred to `sdd-design`

Whether tenant-scoped **child** tables (`pet_media`, `application_events`, …) carry a
denormalised `shelter_id` or use an `EXISTS`-on-parent policy. Recommendation:
denormalise — direct, indexable, and it makes the ADR-0002 test rule apply uniformly.

## Delivery Slices

| Slice | Content | Authored lines (est.) |
|---|---|---|
| 01A | pgx/sqlc/goose setup, roles, `WithTenant`, testcontainers harness, §4.1 tables | ~450 |
| 01B | `media`, `species`, `breeds`, `pets`, pet_* (§4.2–4.3) + indexes + seeds | ~700 |
| 01C | Form templates, immutable versions, submissions (§4.4) | ~400 |
| 01D | Adoption flow, documents, append-only events/audit (§4.5) | ~550 |

**This phase exceeds the 400-line review budget by roughly 5x in total.** 01A/01B/01D
are each still forecast above budget; `sdd-tasks` MUST split them further along the
natural line of *one table group = migration + policy + A/B test*. sqlc output is
generated and excluded from the authored count, but stays in snapshot identity.

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `apps/api/go.mod` | Modified | pgx/v5, goose/v3, testcontainers-go(+postgres), uuid |
| `apps/api/internal/db/migrations/` | New | goose SQL, embedded via `//go:embed` |
| `apps/api/internal/db/` | New | pool, `WithTenant`, sqlc output, test harness |
| `sqlc.yaml` | New | pgx/v5 target |
| `Makefile` | Modified | `migrate`, `db-reset`; `sqlc generate` in `generate` |
| `.github/workflows/` | Modified | generated-code diff check; `-race` job |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Docker daemon unreachable (verified down at exploration) | High | Gate the RLS suite on `testing.Short()`; document `make dev` as a prerequisite; CI is Linux with a live daemon |
| testcontainers-go under `CGO_ENABLED=0` is **assumed, unverified** | Med | Prove it in the very first 01A task, before any migration depends on it. If it fails, fall back to a compose-backed test database |
| **Creating `app_tenant` through the Neon Console / CLI / API silently disables ALL RLS** — see the verified finding below | **Critical** | Roles MUST be created by the goose migration (D1). Add a test asserting the connecting role has neither `rolsuper` nor `rolbypassrls` |
| `uuid.NewV7()` API/version unconfirmed | ~~Low~~ **resolved** | Verified: `func NewV7() (UUID, error)` in `google/uuid`. Returns an error, so it cannot be used in a struct literal without handling it. No fallback needed |
| RLS policy wrong but tests still pass (e.g. connecting as owner) | High impact | Tests MUST connect as `app_tenant`; add one test asserting the owner role is *not* used. Judgment Day before merging the policies |
| `go get` is permission-gated | Med | Batch every dependency into one 01A task so the user is prompted once |

## Verified finding: how Neon can silently defeat every RLS policy

Verified 2026-08-29 against Neon's official documentation. The proposal originally
carried this as an unverified "Neon may restrict `CREATE ROLE`" risk. **The real
behaviour is the opposite, and far more dangerous.**

1. Neon's `neon_superuser` role **carries the `BYPASSRLS` attribute**, which lets its
   members bypass row-level security entirely.
2. That role is **granted automatically to every role created through the Neon Console,
   CLI, or API** — including the default project role (`neondb_owner`).
3. Roles created with **SQL** `CREATE ROLE` do **not** receive that membership. Neon's
   own documentation states that creating roles with restricted privileges is exactly
   what SQL is for, and its RLS guide explicitly says to avoid `neondb_owner` and to use
   a custom role that lacks `BYPASSRLS`.

The failure mode is the worst kind: **provisioning `app_tenant` from the Neon dashboard
would make every policy in this phase a no-op in production, while every A/B isolation
test kept passing locally** — the compose Postgres has no `neon_superuser`, so the
tests would never see it. Silent, environment-specific, and invisible to the suite that
exists to catch precisely this.

Consequences for this proposal:

- **D1 is not merely "workable on Neon" — it is the only safe path.** Roles are created
  by the migration, in SQL, never through the dashboard.
- The A/B test suite gains a **guard assertion**: the role the tests connect as must
  have `rolsuper = false` AND `rolbypassrls = false`, read from `pg_roles`. This is
  cheap and it is the only check that would catch a hand-provisioned Neon role before
  it reaches production.
- Deployment runbooks (Phase 11) MUST state that shelter-facing roles are never created
  from the Neon console.

## Rollback Plan

- **Per migration**: every file ships a tested `-- +goose Down`. `goose down-to <v>`
  reverses a slice. Exception D1b: roles are never dropped (grants revoked only).
- **Per slice**: `git revert` the merge + `goose down-to` the prior version.
- **Data safety**: Phase 01 is greenfield — no deployed code writes these tables, so
  rollback is schema-only with zero data loss. *This holds only while Phase 01 is
  unreleased*; after Phase 02 ships, down migrations become destructive.
- **Local**: `make db-reset` (Makefile-owned; never a manual `rm`).
- **Neon**: apply to a Neon branch first; rollback = delete the branch (*unverified —
  confirm Neon branching before first deploy*).
- **Dependencies**: revert `go.mod` / `go.sum`.

## Dependencies

- Phase 00 complete (verified). Docker daemon running. goose CLI (devcontainer).
- sqlc CLI must be added to `.devcontainer/postCreate.sh` and CI.

## Proposal question round (auto mode — unanswered, needs user review)

1. Is `refresh_tokens` wanted in Phase 01, or held until auth (Phase 02)? Assumed: now.
2. Are 4 chained PRs acceptable, or is a single `size:exception` PR preferred?
3. Should `app_public` policies ship in 01B with `pets`, or as a Phase 06 delta?
   Assumed: 01B, so the read-only role is exercised from the start.

## Success Criteria

- [ ] All 19 §4 tables exist with `ENABLE` + `FORCE ROW LEVEL SECURITY`, and at least one
      policy on every one of them **except `refresh_tokens`**.
      > **Amended 2026-08-29** to resolve a real contradiction found by `sdd-spec`. This
      > criterion originally said "and a policy" for all 19, which design D6 contradicts:
      > `refresh_tokens` is identity-domain and ships **default-deny** this phase — RLS
      > enabled, no policy, no grant — because Phase 02 owns its access path. RLS enabled
      > with zero policies denies everything to a non-owner, so the exception is *more*
      > restrictive, not less. Design wins as the fresher authority; the exception is
      > named here so it stays a declared decision rather than a silent gap.
- [ ] Every tenant-scoped table has a passing A/B test proving tenant B cannot read,
      write, or delete tenant A's rows, run as `app_tenant` (ADR-0002 rule).
- [ ] `app_public` proven read-only and limited to published pets.
- [ ] A guard test asserts the role the suite connects as has `rolsuper = false` AND
      `rolbypassrls = false` in `pg_roles`. Without it, a hand-provisioned Neon role
      would disable every policy in production while the suite stayed green.
- [ ] No `SET LOCAL` anywhere; only `set_config(..., $1, true)`.
- [ ] `goose up` then `goose down-to 0` is clean on a fresh container.
- [ ] `make generate` produces no diff in CI.
- [ ] `go test ./...` green; coverage ≥ 75%; `test-race` green on CI.
