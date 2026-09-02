# Tasks: Phase 01 — Domain and Data

> Implements **FASE-01**. Authority: `design.md` (primary), `specs/*/spec.md`, `proposal.md`.
> Task format mirrors `docs/vault/30-fases/FASE-00.md`. Ids are stable and never reused.
> Mirror of Engram `sdd/phase-01-domain-and-data/tasks`.

States: `[ ]` pending · `[~]` in progress · `[x]` done · `[!]` blocked

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | **~4,112 authored** (sum of the per-task `est:` values below) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | 15 chained PRs across 5 slices (01A → 01E) |
| Delivery strategy | ask-on-risk |
| Chain strategy | **feature-branch-chain** — decided by the user 2026-08-29 |

Decision needed before apply: **RESOLVED 2026-08-29**
Chained PRs recommended: Yes
Chain strategy: **feature-branch-chain**, 15 PRs
400-line budget risk: High

> **User decision, 2026-08-29 (`delivery_strategy: ask-on-risk`).** 15 chained PRs on base
> branch `feature/fase-01`; PR *n* branches from PR *n-1*; the chain merges to `main` as a
> whole. Median PR ~250 authored lines, 13 of 15 under budget.
>
> **PR 4 (455) and PR 5 (424) knowingly exceed the 400-line budget.** They were not split
> further, and that is the accepted cost of this option — but they are also the two PRs
> carrying roles, grants, the `pg_roles` BYPASSRLS guard, the first RLS policies and
> Judgment Day. Reviewers should treat them as the highest-attention PRs of the phase, not
> as ordinary ones that happen to be long. If either grows during apply, split it rather
> than letting it drift further over.

**What is counted.** `est:` is *authored* lines (`additions + deletions`) written by hand:
SQL migrations, Go source, Go tests, YAML, Markdown. **Excluded as generated:** the entire
`internal/db/sqlcgen/` tree and `go.sum`. Both stay inside snapshot identity and receipt
validation; they are excluded only from the 400-line review budget. `go.mod` **is** counted.

**The forecast is roughly double the proposal's.** `proposal.md` estimated ~2,100 authored
lines (~5x budget). Task-level decomposition lands at ~4,112 (~10.3x). The gap is almost
entirely the test suite: 15 table-driven A/B cases, 6 orphan cases, the catalog meta-test,
the append-only suite and the `app_public` suite total ~1,530 lines — the phase's actual
deliverable is *proof of isolation*, and proof is not cheap. Migrations are ~1,380.

### Slice subtotals

| Slice | Content | Tasks | Authored lines | PRs |
|---|---|---|---|---|
| 01A | Deps, spike, `WithTenant`, migration plumbing, roles, harness, §4.1 tables | T-01-001…016 | **1,684** | 5 |
| 01B | `media`, reference data, `pets`, pet children, orphan + public suites | T-01-017…022 | **877** | 3 |
| 01C | Form templates, immutable versions, submissions | T-01-023…026 | **381** | 2 |
| 01D | Adoption flow, notes, documents, append-only events + audit | T-01-027…032 | **705** | 3 |
| 01E | sqlc query inputs, ADR-0007…0011, board backfill | T-01-033…035 | **465** | 2 |
| | | **35** | **4,112** | **15** |

### Suggested Work Units

| Unit | Tasks | Lines | Focused test command | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| PR 1 | 001–002 | 175 | `cd apps/api && go test ./internal/db/dbtest/...` | Docker daemon + `make dev` | revert `go.mod`, delete `internal/db/dbtest/` |
| PR 2 | 003–004 | 305 | `cd apps/api && go test ./internal/db/ -run TestWithTenant` | N/A — stubbed `pgx.Tx`, no database | delete `db.go`, `tenant.go`, `tenant_test.go` |
| PR 3 | 005–007, 012 | 380 | `cd apps/api && go test ./internal/db/ -run TestMigrations && make generate` | N/A — pure `fs.FS` + build wiring | delete `migrate*.go`, `bootstrap.go`, `sqlc.yaml`; revert Makefile/CI |
| PR 4 | 008–011 | 455 | `cd apps/api && go test ./internal/db/rlstest/...` | `make dev` then full suite | `goose down-to 0`; delete `rlstest/` (`00001` now rolls back with PR 3) |
| PR 5 | 013–016 | 424 | `go test ./internal/db/rlstest/ -run 'AB\|Catalog\|Scope'` | `make dev` + Judgment Day | `goose down-to 1`; revert `00002` + its suite rows |
| PR 6 | 017–018 | 242 | `go test ./internal/db/rlstest/ -run 'Media\|Reference'` | `make dev` | `goose down-to 2` |
| PR 7 | 019 | 239 | `go test ./internal/db/rlstest/ -run 'Pets'` | `make dev` | `goose down-to 4` |
| PR 8 | 020–022 | 396 | `go test ./internal/db/rlstest/ -run 'Orphan\|Public\|PetChildren'` | `make dev` | `goose down-to 5` |
| PR 9 | 023–024 | 264 | `go test ./internal/db/rlstest/ -run 'FormTemplate'` | `make dev` | `goose down-to 6` |
| PR 10 | 025–026 | 117 | `go test ./internal/db/rlstest/ -run 'FormSubmission'` | `make dev` | `goose down-to 7` |
| PR 11 | 027–028 | 207 | `go test ./internal/db/rlstest/ -run 'Application'` | `make dev` | `goose down-to 8` |
| PR 12 | 029–030 | 259 | `go test ./internal/db/rlstest/ -run 'AppendOnly\|Notes'` | `make dev` | `goose down-to 9` |
| PR 13 | 031–032 | 239 | `go test ./internal/db/rlstest/ -run 'Documents\|Audit'` | `make dev` | `goose down-to 10` |
| PR 14 | 033 | 200 | `make generate && git diff --exit-code` | N/A — codegen determinism only | delete `internal/db/query/`, `sqlcgen/` |
| PR 15 | 034–035 | 265 | `make lint` | N/A — documentation only | delete the five ADR files |

> **Work units re-balanced 2026-08-29, forced by a compile-time dependency.** `T-01-007`
> (migration `00001`) moved from PR 4 into PR 3, because `//go:embed migrations/*.sql` does not
> compile with no `.sql` file present — verified, and a `.gitkeep` does not satisfy it. A
> boundary that cannot compile on its own is not a boundary. Happily this improves both units:
> PR 3 goes 318 → **380** (still under budget) and PR 4 goes 517 → **455**, so the phase's
> worst over-budget PR shrinks.

For a **feature-branch-chain**: PR 1 base = `feature/fase-01`; PR *n* base = PR *n-1* branch.
For **stacked-to-main**: each PR merges to `main` in order; slice boundaries (PR 5, 8, 10, 13)
are the safe stopping points where the schema is internally consistent.

### Legend for the task metadata lines

- `est:` authored lines, generated output excluded.
- `pilot:` `blacklisted (§7.2)` = never eligible for a free-model pilot (migrations, RLS
  policies, roles, grants, anything security-shaped). `eligible` = ordinary work.
- `parallel:` tasks that may run concurrently with this one.
- `open-q:` a phase open question this task depends on; it MUST NOT be answered silently.

---

## Slice 01A — Foundation, roles, and §4.1 tables

- [x] **T-01-001** · Batch every Go dependency in one `go get`  — **DONE 2026-08-29**
      - spec: schema-migrations / *Migrations are embedded and replayed identically everywhere*
      - deps: `pgx/v5`, `pressly/goose/v3`, `testcontainers-go` + its `postgres` module, `google/uuid`
      - tests: `go build ./...` clean · `gofmt -l` clean · `go vet ./...` clean · `go test ./...` green
      - dod: one permission prompt, not eight · all five present in `go.mod` at pinned versions
      - est: 30 (`go.sum` excluded as generated)
      - **resolved versions:** `pgx/v5 v5.10.0` · `goose/v3 v3.27.3` · `testcontainers-go v0.44.0`
        + `modules/postgres v0.44.0` · `google/uuid v1.6.0` · `CGO_ENABLED=0` is already the
        toolchain default on this host, which is the condition T-01-002 must prove
      - **DoD CORRECTED 2026-08-29 — read this before "fixing" `go.mod`.** The original line
        demanded *"`go.mod` lists all five **direct** requires"* **and** *"`go mod tidy` produces
        no error"*. Those two cannot both hold at this point in the sequence, and the conflict is
        Go's module semantics, not a mistake in the install:
        - A require is `// indirect` until a **source file imports it**. Nothing imports these
          yet — the code that does arrives in T-01-002 (testcontainers), T-01-004 (pgx),
          T-01-006 (goose) and T-01-013 (uuid).
        - `go mod tidy -diff` was run and **confirms it would delete all five**, reverting
          `go.mod` to `chi` alone. Verified, not assumed.
        - **Do NOT run `go mod tidy` until T-01-006 lands.** CI does not gate on it
          (`ci.yml` runs `go build` and `go vet` only), so nothing forces the issue.
        - Rejected: a `//go:build tools` file with blank imports to force them direct. It would
          exist only to make this checkbox true and would have to be deleted again a few tasks
          later. Fixing the criterion beats adding code that lies about why it exists.
      - pilot: eligible
      - engram: —
      - note: **ordering deviation.** The brief asks for the spike first *and* for one batched
        install. The spike cannot compile without `testcontainers-go`, so the batch runs first
        and T-01-002 remains the gate on every downstream task. See Risks.

- [x] **T-01-002** · Spike: prove `testcontainers-go` works under `CGO_ENABLED=0`  — **DONE 2026-08-29**
      - spec: schema-migrations / *The integration suite cannot silently skip itself*
      - **PREREQUISITE — Docker daemon.** Docker Desktop v29.7.2 is installed on this Windows
        host but the daemon was **not running** at last check. `make dev` must be up before this
        task and every later integration task. This is the only task that states it; from here on
        it is assumed.
      - build: `apps/api/internal/db/dbtest/container.go` — `Postgres(t) *Env`, skips on
        `testing.Short()`, **fails** (never skips) when the daemon is unreachable, honours
        `MASCOTAPP_SKIP_DOCKER_TESTS=1`, and `TestMain` fails outright if that variable is set
        while `CI=true`
      - tests: container starts, `select version()` returns PostgreSQL 17, container terminates
      - dod: green on this Windows host with `CGO_ENABLED=0` · **if it fails**, fall back to a
        compose-backed test database as `proposal.md` specifies, and record the failure before
        any migration task starts
      - est: 145
      - **RESULT 2026-08-29 — the spike PASSES. No fallback needed.** `testcontainers-go v0.44.0`
        starts `postgres:17-alpine` under `CGO_ENABLED=0` on this Windows host.
        `server_version_num` in [170000, 180000), container terminated via
        `testcontainers.CleanupContainer`. A second assertion confirms the owner can
        `CREATE EXTENSION citext` in that image (`citext 1.6`), which is what D7 rests on.
        The 33 downstream tasks are unblocked.
      - **Skip semantics verified end to end, not assumed:**
        `-short` → SKIP (not a vacuous pass) · `MASCOTAPP_SKIP_DOCKER_TESTS=1` off CI → SKIP ·
        that same variable with `CI=true` → process FAILS, exit 1.
      - **One property could NOT be exercised, and is pinned structurally instead.**
        "Unreachable daemon FAILS rather than skips" cannot be simulated on a developer host:
        `DOCKER_HOST=tcp://127.0.0.1:1` and `DOCKER_CONTEXT=does-not-exist` were both tried and
        **both ignored** — testcontainers resolves Docker Desktop's named pipe directly and the
        container started anyway. Rather than claim an untested guarantee, the skip decision was
        extracted into the exported pure function `dbtest.SkipReason`, and `TestSkipReason`
        asserts the *negative* property that matters: no daemon-related variable buys a skip.
        `Postgres` has exactly two branches — skip when `SkipReason` says so, `t.Fatalf`
        otherwise — so a startup failure has nowhere else to go.
      - **Two environment findings that affect every later task:**
        1. `go get` on a module root does **not** resolve a subpackage's dependencies.
           `pgxpool` needed `github.com/jackc/puddle/v2`, which arrived only after an explicit
           `go get github.com/jackc/pgx/v5/pgxpool`. T-01-001's batch was incomplete because of
           this, not because a dependency was forgotten.
        2. Windows **Application Control blocks freshly linked test binaries** under
           `%LOCALAPPDATA%\Temp`: *"An Application Control policy has blocked this file"*. The
           build succeeds, the run never starts, and it reads like a test failure. Fixed in the
           Makefile with a repo-local `GOTMPDIR` (`apps/api/.gotmp`, gitignored, removed by
           `make clean-api`). A `go` cleanup error there was observed **once** and did not
           reproduce across three forced uncached runs — recorded as transient, not resolved.
      - pilot: eligible
      - engram: `mascotapp/testing/testcontainers-cgo-disabled`
      - note: this is the phase's single assumed-but-unverified dependency. Nothing downstream
        may start until it is proven or the fallback is in place.

- [x] **T-01-003** · RED — `WithTenant` unit tests against a recording `pgx.Tx` stub  — **DONE 2026-08-29**
      - spec: tenant-isolation / *Tenant scope is transaction-local* (scenarios: rejects an
        absent tenant; never commits after a failure)
      - build: `apps/api/internal/db/tenant_test.go` — stub records `Begin`/`Exec`/`Commit`/`Rollback`
      - tests: `uuid.Nil` → `ErrNoTenant` with **no** `Begin` · `fn` error → rollback, no commit ·
        `fn` panics → rollback **then** re-panic · the `set_config` call passes the UUID as `$1`
      - dod: fails to compile (symbols absent) · no database · `t.Parallel()`
      - est: 160
      - pilot: blacklisted (§7.2)
      - engram: —
      - parallel: T-01-005
      - **RESULT 2026-08-29.** 8 tests in `internal/db/tenant_test.go`, all green, plus the
        `WithTenant` implementation itself (`tenant.go`) — RED was written and observed
        failing first, then GREEN. T-01-004 therefore reduces to pool construction.
      - **CONTRACT REFINED — `*pgxpool.Pool` → `db.Beginner`.** The design's signature took the
        concrete pool, which cannot be stubbed, so every guarantee would have been reachable
        only from an integration test — meaning unverified exactly when the Docker daemon is
        down. `Beginner` is the one method the wrapper calls; `*pgxpool.Pool` satisfies it
        unchanged, so no call site pays for it. Recorded in `design.md`.
      - **VERIFIED BY MUTATION, not by coverage. 6 mutants introduced, 6 killed:**
        M1 drop the `uuid.Nil` check · M2 `set_config(..., false)` (scope leaks to the pooled
        connection) · M4 swallow the panic instead of re-raising · M5 skip the rollback on
        callback failure · M6 run `fn` before the scope is set · M7 interpolate the tenant id
        into SQL text instead of binding it. Every one produced a failing test.
      - **govulncheck went red from the new dependencies and is now clean.** `GO-2026-6253`
        in `moby/go-archive@v0.2.0` was a **symbol-level** hit — reachable from our code — and
        `GO-2026-6303` in `golang.org/x/crypto@v0.54.0` a package-level one. Bumped to
        `go-archive@v0.3.0` and `x/crypto@v0.55.0`. `GO-2026-5932` remains at module level with
        **no fix available** and is not called. `make lint-api` exits 0.
      - lint: `revive` rejected `const max` in `dbtest` as shadowing a builtin — renamed to
        `limit`.

- [x] **T-01-004** · GREEN — pool construction and the `WithTenant` contract  — **DONE 2026-08-29**
      - spec: tenant-isolation / *Tenant scope is transaction-local*
      - build: `internal/db/db.go` (`NewPool`, `NewPublicPool`, `Close`, pool sizing, `ConnConfig`) ·
        `internal/db/tenant.go` (`WithTenant`, `WithPublic`, `ErrNoTenant`)
      - tests: T-01-003 turns green
      - dod: `SELECT set_config('app.shelter_id', $1, true)` strictly inside `Begin`…`Commit` ·
        **no `SET LOCAL` anywhere** · GUC never set in `AfterConnect`/`BeforeAcquire` ·
        the pool is never handed to `fn` · `WithPublic` opens `BEGIN READ ONLY` with no GUC
      - est: 145
      - pilot: blacklisted (§7.2)
      - engram: `mascotapp/db/withtenant-contract`
      - **RESULT 2026-08-29.** `internal/db/db.go` (`PoolConfig`, `PublicPoolConfig`, `NewPool`,
        `NewPublicPool`) and `WithPublic` in `tenant.go`. 9 further unit tests, no database.
      - **The pool configuration is a COST contract, and the tests say so.** `free-tier-limits.md`
        names Neon's 100 CU-hours/month as the single most exhaustible resource in the stack
        (~3.3 h/day). Verified against Neon's docs: the free plan suspends the compute after
        **5 minutes** of inactivity and **cannot disable** Scale to Zero, and Neon closes idle
        connections itself. So:
        - `MinConns` and `MinIdleConns` are pinned to **0** and asserted. One warm connection
          would keep the compute from ever suspending and spend the month's entire budget on an
          idle database.
        - `MaxConnIdleTime` is **2 min**, asserted to stay below `NeonAutoSuspendAfter`.
          pgxpool's default is **30 minutes** — six times the window — which would leave sockets
          in the pool that Neon had already dropped.
        - `NeonAutoSuspendAfter` has its own test pinning it to 5 minutes, so raising it to make
          another test pass fails loudly instead.
      - **One hypothesis checked and DISPROVED before acting on it.** I suspected pgxpool's
        1-minute background health check was a keep-alive that would defeat autosuspend — the
        exact trap Neon's own Ecto guide describes for Postgrex's `idle_interval`. Reading
        `checkConnsHealth` in v5.10.0 shows it only inspects local state (idle duration, expiry)
        and destroys connections locally, with **no network round trip**. Left at its default;
        no change made on a guess.
      - **VERIFIED BY MUTATION. 5 mutants introduced, 5 killed:** P1 `MinConns = 1` ·
        P2 `MaxConnIdleTime` back to pgxpool's 30 min · P3 drop `default_transaction_read_only`
        from the public pool · P4 `WithPublic` opens read-write · P5 `WithPublic` sets a tenant
        scope (which would silently narrow the public catalog to one shelter).
      - **`WithPublic` takes `db.TxBeginner`**, the `BeginTx` counterpart of `Beginner`, for the
        same reason: the read-only guarantee is asserted without a container.
      - **The `.gotmp` flake is diagnosed and fixed — my earlier reading of it was wrong.**
        Relocating `GOTMPDIR` into the repository was not the fix; the failures returned there,
        reproducibly, three runs in a row, in two forms (`An Application Control policy has
        blocked this file` and `The process cannot access the file because it is being used by
        another process`). The real cause is **stale build directories accumulating in the
        scratch dir**: once one holds a blocked or locked binary, every later run inherits it.
        `make test-api` and `make test-short` now **wipe** `.gotmp` before each run. Validated
        across 4 cached runs and 3 forced fresh links: 0 incidents. `GOCACHE` is untouched, so
        nothing is recompiled needlessly.

- [x] **T-01-005** · RED — embedded migration set invariants, no database  — **DONE 2026-08-29**
      - spec: schema-migrations / *Embedded set is non-empty and well-formed* ·
        *No password literal in the migration set*
      - build: `apps/api/internal/db/migrate_test.go`
      - tests: FS non-empty · versions strictly ascending, no gaps, no duplicates · every `Up`
        has a `Down` · **no `PASSWORD` literal in any file** · the roles migration has a strictly
        lower version than the first migration that enables RLS
      - dod: pure Go over the embedded `fs.FS` · runs under `-short`
      - est: 90
      - pilot: blacklisted (§7.2)
      - engram: —
      - parallel: T-01-003
      - **RESULT 2026-08-29.** `internal/db/migrate_test.go`, 6 tests, pure Go over the embedded
        `fs.FS`, no database. Observed RED: `undefined: db.Migrations`.
      - Two invariants added beyond the brief, both catching silent failures:
        - **`ENABLE` without `FORCE`.** Every table that enables RLS must also force it.
          Migrations run as the owner, and without `FORCE` the owner bypasses every policy —
          the exact classic mistake ADR-0002 exists to prevent.
        - **Roles strictly before the first RLS.** A policy names the role it applies `TO`, so a
          table enabling RLS before `app_tenant` exists either fails to migrate or ends up with
          RLS on and no policy for a role that arrives later.
      - **BLOCKER FOUND FOR T-01-006 / T-01-007 — verified, not assumed.** `//go:embed
        migrations/*.sql` is a **compile error** when no `.sql` file matches
        (`pattern migrations/*.sql: no matching files found`), and a `.gitkeep` does not help
        because embed ignores dotfiles. Both were reproduced in a throwaway module. So
        `migrate.go` cannot compile until the first migration exists — which means **T-01-007
        must land in the same PR as T-01-006.** See the work-unit change below.
- [x] **T-01-006** · GREEN — migration embedding and password bootstrap  — **DONE 2026-08-29**
      - spec: schema-migrations / *Role passwords never appear in migration text*
      - build: `internal/db/migrate.go` (`//go:embed migrations/*.sql`, `Migrations() fs.FS`,
        `Up`, `DownTo`, `Version`) · `internal/db/bootstrap.go` (`ValidateRoleCredentials`,
        `SetRolePassword`, D8)
      - tests: T-01-005 turns green
      - dod: bootstrap runs **outside goose**, as owner, value bound as `$1` and quoted
        server-side by `format('%L')` — never concatenated into SQL text
      - est: 85
      - pilot: blacklisted (§7.2)
      - engram: —

- [x] **T-01-007** · Migration `00001_extensions_roles_and_grants.sql`  — **DONE 2026-08-29**
      - spec: schema-migrations / *Roles are provisioned by migration and never dropped* ·
        data-model-core / *Email comparison and uniqueness are case-insensitive by type*
      - build: `CREATE EXTENSION IF NOT EXISTS citext` as owner, FIRST — the only extension in
        this schema (D7, reversed 2026-08-29) · then `app_tenant` and `app_public` via `DO`
        guarded on `pg_roles`; `NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS NOINHERIT LOGIN`,
        no password · `REVOKE ALL ON SCHEMA public FROM PUBLIC` · `GRANT USAGE` to both
      - tests: replay is a no-op · `Down` revokes grants and the roles still exist in `pg_roles`
        (D1b) · `Down` leaves `citext` present in `pg_extension`
      - dod: `Down` **never** drops a role, and **never** drops the extension —
        `DROP EXTENSION citext` would cascade to `users.email` · `pg_extension` contains `citext`
        and **nothing else**
      - est: 62
      - pilot: blacklisted (§7.2)
      - engram: `mascotapp/db/roles-by-migration-only`
      - **RESULT 2026-08-29 — T-01-006 and T-01-007 landed together**, as the compile-time
        dependency found in T-01-005 required. 6 further unit tests plus **7 integration tests
        that run the migration against a real container** — the unit tests only inspect text,
        and shipping SQL that has never executed is not shipping it.
      - Verified against a live PostgreSQL 17: `goose up` applies · `up → down-to 0 → up` is
        clean · `app_tenant` and `app_public` exist with `rolsuper = false`,
        `rolbypassrls = false`, `rolcanlogin = true` · `pg_extension` holds **exactly** `citext`
        · `SetRolePassword` works with a password containing a quote and a backslash, and the
        stored value is not the plaintext.
      - **goose is driven through `NewProvider`**, not the package-level `SetBaseFS`/`SetDialect`,
        which mutate global state; this phase's suite migrates several containers in parallel.
      - **A design error of mine, caught before it shipped.** The first `bootstrap.go` put
        `SELECT set_config(...); DO $$...$$;` in one `ExecContext` with `$1`. PostgreSQL's
        extended protocol carries **one statement per parameterised call**, so it could never
        have worked — and since `set_config(..., true)` is transaction-local, the statements must
        also share one transaction, which a pooled `*sql.DB` does not guarantee.
        `SetRolePassword` now opens its own transaction rather than documenting that the caller
        should: a failure that depends on pool timing is the worst kind to leave to a convention.
      - **VERIFIED BY MUTATION. 6 introduced, 5 killed immediately, 1 SURVIVED.** Killed:
        B1 concatenate the password with `Sprintf` · B2 accept any role name (`postgres`
        included) · B3 `set_config(..., false)` · M8 create `app_tenant` **with** `BYPASSRLS` ·
        M9 drop `CREATE EXTENSION citext`.
      - **The gap: M10, `Down` dropping the roles, SURVIVED.** The round-trip test does
        `Up → DownTo(0) → Up` and passed anyway, because the second `Up` recreated whatever the
        `Down` destroyed. **Nothing looked at the state between them.** It matters:
        `CREATE ROLE` is cluster-wide, so a `goose down-to` on one database would break a second
        database in the same cluster that is using the role. Closed with
        `TestMigrations_DownKeepsRolesAndExtension` — after the rollback both roles still exist,
        `citext` is still installed, **and** the schema grant is gone. M10 now dies, and so does
        a new M11 (`Down` dropping the extension, which cascades to `users.email`).
      - **Rule this makes explicit:** a round-trip test that ends where it started cannot see
        what happened in the middle. Assert the intermediate state, or the two halves cover for
        each other.
      - **`go mod tidy` run**, as T-01-001 said to defer until now. All five dependencies are
        imported and were promoted to direct requires — exactly what T-01-001's original DoD
        asked for, at the point where it became true rather than before.
      - lint: `gosec` G101 flagged two SQL constants as hardcoded credentials, a name heuristic
        firing on identifiers containing "password". Suppressed with a narrow, justified
        `//nolint` pointing at the test that asserts the stronger property. 0 issues.

- [x] **T-01-008** · **Role guard** — the connecting role cannot bypass RLS  — **DONE 2026-08-30**
      - spec: tenant-isolation / *The connecting role cannot bypass RLS* (both scenarios)
      - build: `internal/db/dbtest/roles.go`, asserted in harness setup so **every** table test
        inherits it · extend `dbtest.Env` with `TenantPool` and `PublicPool`
      - tests: `SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user`
        returns `(false, false)` for `app_tenant` **and** `app_public` · `current_user` is not
        the owner role · pointing the harness at a `BYPASSRLS`/`SUPERUSER` role fails at setup
      - dod: the guard runs in harness setup, not as a standalone test a filter can skip
      - est: 55
      - pilot: blacklisted (§7.2)
      - engram: `mascotapp/security/neon-rls-bypass`
      - note: partially covered already — `TestMigrations_CreateRolesThatCannotBypassRLS` asserts
        the attributes on the created roles. What remains is asserting them for the role the
        suite **connects as**, which is the case a hand-provisioned Neon role would break.
      - **RESULT 2026-08-30.** `internal/db/dbtest/roles.go` — `RoleCapabilities`,
        `ReadRoleCapabilities`, the pure `CheckRoleCannotBypassRLS`, and `guardPool`. `Env` now
        carries `OwnerRole`, `TenantPool` and `PublicPool`; `Postgres` migrates the container,
        sets both role passwords through `db.SetRolePassword` (the same path production uses,
        since D8 keeps them out of the migration set), builds a pool **as each application
        role**, and guards it before handing it back.
      - The guard reads `current_user`, not a role name we pass in. The question that matters is
        not "is `app_tenant` configured correctly" but "can the role this connection is actually
        using bypass the policies these tests are about to assert".
      - **Proven against a role that genuinely can bypass.** The container's owner is a
        superuser, so `TestGuard_RefusesARoleThatReallyCanBypassRLS` reads its real capabilities
        and requires the guard to refuse it — and fails loudly if the owner ever stops being able
        to bypass, since the test would then prove nothing.
      - **VERIFIED BY MUTATION. 5 introduced, 4 killed immediately, 1 SURVIVED.** Killed:
        G1 accept `BYPASSRLS` · G2 accept `SUPERUSER` · G3 accept the owner · G5 build the
        application pools with the owner's credentials.
      - **The gap: G4, the harness not calling the guard at all, SURVIVED — and G4 is this
        task's entire DoD.** Deleting `guardPool` from `appPool` broke nothing, because the only
        test exercising the guard called `CheckRoleCannotBypassRLS` directly. The check was
        correct and **unenforced**. Closed by making the guard **observable**: `guardPool` returns
        what it verified, `Env` records it, and
        `TestPostgres_GuardsEveryApplicationPoolDuringSetup` asserts both roles are in that
        record. G4 now dies.
      - **Rule this makes explicit:** testing a check is not testing that the check runs. If the
        guarantee is "this happens during setup", something has to observe that it happened —
        otherwise the guarantee is a comment.
      - lint: `revive` `context-as-argument` on three helpers taking `(t, ctx, ...)`. Reordered
        to `(ctx, t, ...)` rather than suppressed — the rule is right, the code was wrong.
      - **KNOWN DEVIATION FROM DESIGN, carried to T-01-009.** `design.md` says *"Postgres starts
        one container per test package"*; the implementation starts **one per test**. With
        `go test ./...` running packages in parallel that is roughly 14 containers at once, which
        is why these two packages take ~20s each. One transient `internal/db` failure was
        observed under `make test-api` and did **not** reproduce across three forced uncached
        runs — its message was not captured, so it is recorded as unexplained rather than
        diagnosed. T-01-009 owns the harness and should move to a per-package container.

- [x] **T-01-009** · A/B isolation harness and tenant fixtures  — **DONE 2026-08-30**
      - spec: tenant-isolation / *Tenant B cannot read, write or delete tenant A's rows*
      - build: `internal/db/dbtest/tenants.go` — `ShelterA`/`ShelterB` fixtures, the
        table-driven A/B runner, and the per-table case struct
      - tests: the runner itself, exercised against the first tenant table
      - dod: adding a table is one struct literal · every assertion runs as `app_tenant`,
        never as the owner
      - est: 220
      - pilot: blacklisted (§7.2)
      - engram: —
      - note: **`pet_media`'s row identity is a pair** (`pet_id`, `media_id`) — decided in
        `sdd-tasks`, since §4.3 gives it no `id`. Size that into the runner here rather than
        special-casing it in T-01-020.
      - **RESULT 2026-08-30.** `internal/db/dbtest/tenants.go` — `RowKey`, `TenantTable`,
        `Check`, `RunAB`. Adding a table to the suite is one struct literal, which is the point:
        ADR-0002 makes an A/B test the completion rule for every tenant table, and a rule that
        is expensive to follow is a rule that gets skipped.
      - `RowKey` is a column map, not an id, so `pet_media`'s composite identity needs no
        special case. Asserted with a two-column table.
      - **The runner is proven to FAIL, not just to pass.** Every table in this phase is declared
        safe on its word, so a runner nobody has watched fail is an assumption in a test's
        clothing.
      - **VERIFIED BY MUTATION, in three rounds, and the first two found real gaps.**
        - Round 1: R3 (accept any error instead of `42501`) and R5 (assert as the owner) died.
          **R1, R2 and R4 survived** — the read check, the forged-insert check and the
          "tenant A still sees its own row" check could each be deleted with no test failing.
          Cause: a single permissive table trips several checks at once, and the test only
          asserted *some* error. **One broken table proves that at least one check works, not
          that each one does.** Fixed with one broken table per check, each asserting the
          failure message names the check that fired.
        - Round 2: **R6 survived** — deleting the qualified-UPDATE assertion changed nothing.
          Investigating that produced the real finding below.
        - Round 3: R1, R2, R4, R7 die. **R8 survived** — making the destructive probe commit
          instead of roll back broke no test, because on a correct table it deletes nothing.
          Closed with a test that reads the broken table as the owner afterwards and requires
          the rows to still be there.
      - **REAL DEFECT IN THE RUNNER, found by mutation and confirmed in PostgreSQL's docs.**
        A `DELETE ... WHERE` can never detect a permissive DELETE policy. PostgreSQL 17
        documents that *"because DELETE commands often need to read data from columns (such as
        in a WHERE or RETURNING clause), SELECT rights are typically required... the appropriate
        SELECT or ALL policies are applied **in addition to** the DELETE policies"*, and the same
        holds for UPDATE. So while the SELECT policy is correct, a completely broken DELETE
        policy is unreachable through a qualified statement — and that is also the shape of the
        real attack: **`DELETE FROM t` with no WHERE reads no columns, so the SELECT policy never
        applies and tenant B destroys tenant A's rows without ever being able to see them.**
        Added `checkTenantBCannotWipeTheTable`, which is the only assertion in the runner that
        can catch it. The probe always rolls back.
      - The qualified UPDATE/DELETE assertions are kept as defence in depth and documented as
        such — they cannot fire while the SELECT policy is right, and pretending otherwise would
        be the same false comfort this phase exists to remove.
      - **CARRIED DEVIATION FROM T-01-008 RESOLVED.** `Postgres` now returns **one shared,
        already-migrated container per test package**, as `design.md` always specified, instead
        of one per test. `PostgresIsolated` exists for the two migration tests that run
        `goose down-to 0` and would otherwise pull the floor out from under their neighbours.
        Runtime: `internal/db` ~21s → **~8s**, `dbtest` ~19s → **~5s**.

- [x] **T-01-010** · **Catalog meta-test** — no table escapes classification  — **DONE 2026-08-30**
      - spec: tenant-isolation / *Every table is classified and protected*
      - build: `internal/db/rlstest/catalog_test.go`
      - tests: every relation of kind `r` in `public` is in exactly one of the tenant set (15),
        the non-tenant model set (4), or the infrastructure exemption (`goose_db_version`, which
        goose creates rather than a migration) · all 19 model tables have `relrowsecurity` **and**
        `relforcerowsecurity` true · ≥1 policy on each **except `refresh_tokens`**
      - dod: forgetting a table is a **test failure**, not a silent gap
      - est: 110
      - pilot: blacklisted (§7.2)
      - engram: —
      - note: `relrowsecurity` is asserted on the non-tenant tables too. A policy on a table
        whose RLS is not enabled is **inert**: `users` is protected only by D6's `EXISTS` policy,
        so exempting it would expose every user row while the policy still read as correct.
      - **RESULT 2026-08-30.** `internal/db/rlstest/catalog.go` holds `Schema`, the single
        canonical declaration of every relation this phase creates, and the pure rules over it;
        `catalog_test.go` holds the assertions. The declaration is shared with the A/B suite and
        the child-orphan suite so no second, divergent list can appear later.
      - **The declaration is checked against ITSELF before it is checked against a database.**
        `Validate` refuses a table classified in two sets, a duplicate inside one set, a
        `TenantChildren` entry that is not a tenant table, and a `Pending` or `NoPolicy` entry for
        a table nobody declared. Every rule below is only as good as those lists.
      - `ClassifyAll` is the exhaustiveness rule over what EXISTS, and reports **every** offender
        rather than the first: a rule that surfaces one missing table per run turns a five-table
        migration into five round trips.
      - **Anti-vacuity ledger — the design decision worth arguing about.** The 19 model tables do
        not exist yet, so a protection assertion over them is a no-op, and a green suite would
        read as "19 tables verified" while verifying none. `Pending` names each absent table and
        the task that creates it, and `CheckPending` fails in **both** directions: a pending table
        that has landed (delete its line, which switches its assertions on) and a declared table
        that is absent without a ledger entry. The meta-test logs `verified N of 19 declared model
        tables; M still pending`. Today that reads **0 of 19**, out loud, which is the point.
      - **Every migration task from T-01-013 on must delete its own `Pending` lines.** That is not
        bookkeeping: it is what turns each new table's protection from declared to asserted.
      - Reads `relkind IN ('r','p')`, one kind wider than the spec's `r`. Partitioning a table
        later would otherwise make it vanish from this test — the exact silent gap the DoD forbids.
      - Read as the **owner**: this is introspection, not an isolation assertion, and a role that
        could not see the whole catalog would report a partial schema as a complete one.
      - **VERIFIED BY MUTATION, three rounds, 17 mutants, and the first two found real problems.**
        - Round 1 (15/16): **C5 survived** — deleting the `duplicates()` loop broke nothing,
          because the cross-set `seen` map already catches an intra-set duplicate. The helper was
          **dead code masquerading as a check**. Collapsed to one check, with a message that
          distinguishes the two shapes instead of reading "declared in both Tenant and Tenant".
          `duplicates()` now covers only `TenantChildren`, which the `seen` map does not visit, and
          a case was added for it.
        - Round 2 (15/17): **C5a survived** — dropping the intra-set branch still produced *an*
          error, and the test only asserted `err != nil`. Same lesson as T-01-009: added a `wants`
          fragment per case so each one proves the check it breaks is the one that fired.
        - Round 3: **17/17 killed.**
      - The harness carries the applied-check from T-01-009 (`str.replace` no-ops silently, and an
        unapplied mutation is indistinguishable from a surviving one). C3's anchor went stale after
        the round-1 fix and was correctly reported `NOT-APPLIED` rather than as a kill.
      - suite: `go vet` clean · `golangci-lint` 0 issues · `govulncheck` 0 vulnerabilities ·
        `internal/db/rlstest` ok 6.4s

- [x] **T-01-011** · Migration round-trip: up → down-to 0 → up  — **DONE 2026-08-30**
      - spec: schema-migrations / *Migrations are reversible and idempotent*
      - build: extend `internal/db/migrate_integration_test.go`
      - tests: clean on a fresh container · versions strictly ascending · roles survive the
        rollback (D1b) · `citext` survives the rollback
      - dod: run after every new migration lands, not only once
      - est: 70
      - pilot: blacklisted (§7.2)
      - engram: —
      - note: largely delivered early with T-01-006/007. What remains is re-running it as each
        later migration is added, and asserting the intermediate state after every `Down`.
      - **RESULT 2026-08-30.** `internal/db/migrate_roundtrip_test.go`. **Deviation from the
        build line, stated rather than hidden**: a new file instead of extending
        `migrate_integration_test.go`, which would otherwise mix password-bootstrap assertions
        with rollback assertions in one 400-line file. Same package, so `loadMigrations` and
        `openSQL` are reused.
      - **The gap this closes.** `up → down-to 0 → up` only ever inspects the two endpoints.
        With one migration those are the same journey; with twelve, a `Down` that drops a policy
        and keeps its table, or drops a parent and orphans a child, leaves the schema open at an
        intermediate version and the round-trip never looks. Same blind spot as M10 in T-01-007,
        one level up.
      - `stepwiseWalk` descends **one migration at a time**, driven by the embedded set, so every
        migration this phase adds is covered without anyone remembering to extend it — that is
        the DoD. At each stop it verifies the reported version, then runs the invariants; then it
        climbs all the way back, which is what makes the descent a rollback rather than a
        one-way trip.
      - Invariants at **every** version, not only at 0: `checkDownState` (both roles and `citext`
        survive — D1b) and `checkCatalogAt` (the T-01-010 exhaustiveness and protection rules).
        `CheckPending` is deliberately excluded: halfway down the chain a declared table is
        absent for a legitimate reason, so asserting it would fail for being correct.
      - Also new and detecting something **today**: `checkRecordedSet` compares what goose
        actually recorded against what the binary embeds. The unit tests assert that migration
        FILENAMES ascend; nothing asserted that goose applied all of them, in that order, and
        nothing else. An embedded-but-skipped migration left the unit tests green.
      - **VERIFIED BY MUTATION, four rounds, and the first three found real problems.**
        - Round 1 (**6/15**): nine survivors, all assertions inside the integration test body.
          Cause: **with one migration the walk has one step and nothing to detect**, so deleting
          its assertions changed nothing. Not an artifact — the logic was genuinely unproven.
          Fixed by extracting `stepwiseWalk` behind a `migrator` interface and driving it with a
          fake that records the exact `DownTo` sequence.
        - Round 2 (**16/19**): W7 survived because the fake always climbed back to HEAD; added an
          `upTo` case for a climb that stops short. W17/W18 survived because the recorded and
          embedded lists both had one element and were trivially equal; extracted
          `checkRecordedSet` as a pure rule with five cases.
        - Round 3 (**18/20**): **W14 survived, and it was a real defect in the test.** Both
          `ClassifyAll` and `CheckProtection` answer an unclassified relation with
          `ErrUnclassifiedRelation`, so disabling the exhaustiveness rule left the test green —
          the protection rule covered for it. **The sentinel alone cannot say which rule fired.**
          Same lesson as T-01-009 R1/R2/R4, third time this phase. Fixed by asserting the message
          fragment per case.
        - Round 4: **19/20.**
      - **One accepted survivor, recorded rather than papered over.** W18b deletes the single
        assertion from the leaf integration test that calls `checkRecordedSet`. Nothing observes
        a leaf test, so nothing can catch that; the *rule* is proven (W17/W18 die), the *wiring*
        is not. Closing it would need a meta-test per test. The T-01-008 `GuardedRoles` treatment
        was warranted there because the guard was supposed to run in shared setup for every test;
        it is not warranted for one call in one test.
      - lint: `currentVersion` went dead when the walk was extracted — deleted rather than
        suppressed. Three `unused-parameter` on the fake's `ctx` renamed to `_`.
      - suite: `go vet` clean · `golangci-lint` 0 issues · `govulncheck` 0 vulnerabilities ·
        `internal/db` ok 15.2s (one more isolated container: the stepwise walk destroys the
        schema, so it cannot share the package container)

- [x] **T-01-012** · Build wiring: sqlc, Makefile, CI, devcontainer, env  — **DONE 2026-08-30, one item deliberately deferred**
      - spec: schema-migrations / *Generated code is committed and verified*
      - build: `apps/api/sqlc.yaml` (`sql_package: pgx/v5`, out `internal/db/sqlcgen`,
        `emit_interface: true`, uuid override) · `Makefile` targets `migrate`, `migrate-down`,
        `db-reset` · `sqlc generate` added to `generate` · CI generated-code diff check ·
        `sqlc` CLI in `.devcontainer/postCreate.sh` · `.env.example` entries
      - tests: `make generate` produces no diff in CI
      - dod: `make db-reset` owns local teardown — never a manual `rm`
      - est: 88
      - pilot: eligible
      - engram: —
      - note: `.env.example` is denied to agents by a global settings rule; hand the user the
        exact lines to paste, as was done for `TRUSTED_PROXIES`.
      - **RESULT 2026-08-30.** `apps/api/sqlc.yaml`, three Makefile targets, the sqlc pin in
        `.devcontainer/postCreate.sh`, and `internal/db/wiring_test.go` — build wiring is a
        contract, and it fails the way every untested contract fails: quietly.
      - **DEFERRED, WITH A GUARD: `sqlc generate` does not join `make generate` here. It joins
        at T-01-033**, the task that writes the query files. *(Corrected 2026-08-30: this line
        said T-01-013, which is where the first TABLE lands, not the first QUERY. The body of
        the note already said twenty-one tasks — 012 + 21 = 033 — so the headline was the slip.
        Left as a correction rather than a silent edit: it would have sent the next agent to
        wire sqlc in the wrong task, and T-01-013 confirmed the guard is still green with the
        query directory empty.)* Verified against sqlc v1.31.1: an empty queries directory is a
        hard failure — `error parsing queries: no queries contained in paths`. The first query
        needs the first table, so wiring it at T-01-012 breaks the build for twenty-one tasks.
        A placeholder query to keep the target green would be **code that lies about why it
        exists**, the same reason a `//go:build tools` file was rejected in T-01-001.
        `TestSQLC_IsWiredIntoGenerateExactlyWhenQueriesExist` makes the deferral impossible to
        forget: the moment a `.sql` lands in `internal/db/query/`, it goes RED until the
        Makefile, CI **and** the devcontainer are all wired. **That is T-01-033's RED, built
        here.**
      - `sqlc.yaml` was **run, not just written**. Generated against a realistic `shelters`
        migration in a scratch tree: `ID uuid.UUID` (the override lands), `Querier` emitted, and
        the header carries `// Code generated by sqlc. DO NOT EDIT.` — so golangci-lint v2's
        default `generated: lax` exclusion already covers the output and no lint config is
        needed. `schema:` points at `internal/db/migrations`, the same directory goose applies
        and `//go:embed` ships: a separate `schema.sql` would be a second source of truth.
      - Makefile: `migrate`, `migrate-down` (**one step**, so the blast radius reads as small as
        it is), and `db-reset`, which tears down **through the migration set** — reviewed SQL —
        rather than through `rm` or `docker volume rm`, whose blast radius is whatever the path
        expands to. **`DATABASE_URL` has no default, deliberately**: a migration target that
        works without anyone choosing a database is one shell export away from migrating the
        wrong one. Verified: `make migrate` with it unset exits 1 with an actionable message.
      - New invariant this task's own targets created: `TestMigrations_OnDiskSetMatchesTheEmbeddedSet`.
        The CLI reads the directory, the application runs the embed; every pre-existing check
        read only the embed, so a file the pattern misses was invisible.
      - `TestDevcontainerInstallsEveryToolTheMakefileInvokes` derives the requirement **from the
        Makefile**, so a target added later brings its own. It is not hypothetical: the three new
        targets shell out to `goose`.
      - `TestPinnedToolVersionsAgreeBetweenDevcontainerAndCI` — the generated header records the
        sqlc version, so a devcontainer/CI skew fails the diff check for a reason nobody can
        reproduce locally.
      - **VERIFIED BY MUTATION against the WIRING FILES, not the tests** — for assertions that
        read config, the honest mutant breaks the config. Two rounds, 20 mutants.
        - Round 1 (**15/16**): every sqlc and Makefile mutant died. **P2 survived** — deleting
          the sqlc install broke nothing. Investigating it found a real gap: nothing required the
          devcontainer to install the tools the Makefile runs, and this task had just added three
          targets that run `goose`. Added the rule; P3–P6 now die.
        - Round 2: **19/20.**
      - **One accepted survivor, recorded rather than papered over.** P2 still survives: sqlc is
        genuinely not invoked by anything yet, so no rule can honestly require its install today.
        The deferral guard requires it the moment a query file lands.
      - lint: 0 issues · `go vet` clean · `govulncheck` 0 vulnerabilities · suite green
        (`internal/db` 12.5s). `gopkg.in/yaml.v3` promoted from indirect to direct — already in
        `go.sum`, no new supply chain.
      - **Also corrected here:** the `Makefile` comment claimed wiping `.gotmp` was the fix for
        the Windows Application Control block. It is **not** — the block recurred against a
        freshly wiped directory in T-01-009, T-01-011 and T-01-012. Both earlier diagnoses were
        wrong and the comment now says so instead of leaving a confident explanation that does
        not hold.
      - **OPEN, needs the user:** `.env.example` is denied to agents by a global settings rule.
        The exact lines to paste were handed over at close.

- [x] **T-01-013** · Migration `00002_tenancy_identity.sql` — `shelters`, `users`, `memberships`, `refresh_tokens`  — **DONE 2026-08-30**
      - spec: data-model-core / *Core tenancy and identity tables exist*, *Email comparison and
        uniqueness are case-insensitive by type*, *A user belongs to many shelters* ·
        tenant-isolation / *Grants are explicit and default-deny*
      - RED first: add `shelters` and `memberships` rows to the A/B suite and to the meta-test
        tenant list → fails, relation does not exist. Do not split finer (design §Testing).
      - build: 4 tables, PKs `uuid NOT NULL` with **no** default (D3) · RLS `ENABLE` + `FORCE` on
        all four · `tenant_isolation` on `shelters` (scoped by `id`) and `memberships` ·
        `member_visible_users` `EXISTS`-through-`memberships` on `users` (D6, the single
        sanctioned carve-out) · `refresh_tokens`: RLS on, **no policy, no grant** ·
        `users.email citext NOT NULL UNIQUE` (D7, reversed 2026-08-29 — `citext` verified present
        on Neon AND in `postgres:17-alpine`) · `UNIQUE (user_id, shelter_id)` on `memberships` ·
        `UNIQUE (id, shelter_id)` on `shelters` for later composite FKs · per-table grants only —
        never `ON ALL TABLES`, never `ALTER DEFAULT PRIVILEGES` (D9)
      - dod: `pg_extension` still contains `citext` and nothing else
      - est: 214
      - pilot: blacklisted (§7.2)
      - engram: —
      - ~~open-q: `CITEXT` vs `lower(email)`~~ **RESOLVED 2026-08-29 by the user: `citext`.**
        D7's premise ("Neon's allow-list is unconfirmed") was verified FALSE.
      - open-q: **`refresh_tokens` access path** — default-deny this phase; Phase 02 chooses a
        dedicated `app_auth` role or a user-scoped `app.user_id` GUC.
      - open-q: **who may write a `users` row** — `app_tenant` gets `SELECT` only in Phase 01.
        The write path belongs to Phase 02's registration flow. Grant `SELECT` and stop.
        **RESOLVED as written.** Both layers say it: the policy is `FOR SELECT`, the grant is
        `SELECT`, and `TestTenancyPolicies_ApplyToTheRightRoleAndCommand` pins both.
      - **RESULT 2026-08-30.** `00002_tenancy_identity.sql` (4 tables, 3 policies, 3 grants),
        the A/B suite `internal/db/rlstest/isolation_test.go`, `TenantColumn` and `Fixture` on
        `dbtest.TenantTable`, and the four `Pending` lines deleted from `rlstest/catalog.go` —
        which is what moved the ledger from **0 of 19 verified to 4 of 19**.
      - **The task text asked for `UNIQUE (id, shelter_id)` on `shelters`. That constraint
        cannot exist**: `shelters` has no `shelter_id` column, because its rows ARE the tenants
        and its policy scopes on `id`. The line is a copy of the generic composite-FK rule; no
        other artifact carries it (design §Migration plan lists only the tables and policies for
        `00002`, and every real composite parent — `media`, `pets`, `form_templates`,
        `adoption_applications` — declares it in its own migration). Nothing is lost: the
        property a composite FK needs is "the pair (id, tenant) is unique", and for `shelters`
        that pair is `(id, id)`, already carried by the primary key. **Not implemented, by
        decision.**
      - `verified_by` gets a real FK to `users (id)` — hence `users` is created first in the
        file. `logo_media_id` and `cover_media_id` do **not**: `media` is `00003` and a
        constraint cannot reference a table that does not exist.
        `TestShelters_MediaColumnsGetTheirForeignKeyWhenMediaLands` asserts the equivalence in
        both directions, so **T-01-017's RED is built here** — the same deferral-with-a-guard
        shape T-01-012 used for sqlc.
      - **Found by the T-01-011 round-trip, not in review:** `member_visible_users` lives ON
        `users` but READS `memberships`, and PostgreSQL records that as a dependency, so the
        `Down` failed with `2BP01` on `DROP TABLE memberships`. Fixed with an explicit
        `DROP POLICY member_visible_users ON users;` first — **not** `DROP TABLE ... CASCADE`,
        whose blast radius is whatever the schema happens to contain on the day.
      - **The D8 password guard fired on `users.password_hash`.** It was a substring match on
        `PASSWORD`. Narrowed to a word-boundary regex (`_` is a word character, so it catches
        `ALTER ROLE ... PASSWORD 'x'` and not `password_hash`) and given its own table-driven
        test, `TestHasPasswordClause` — a security rule that gets loosened owes proof of what it
        still catches, or the next person widens it until it catches nothing.
      - **VERIFIED BY MUTATION, three rounds, 21 mutants, and rounds 1 and 2 each found a real
        problem.** The subject is the **migration SQL**, per T-01-012's rule that for an
        artifact-driven test the honest mutant breaks the artifact.
        - Round 1 (**14/20**): **M3 survived** — retargeting `member_visible_users` from
          `app_tenant` to `app_public` broke nothing, because the catalog meta-test *counts*
          policies and cannot see who a policy is FOR. `users` would have been readable by
          nobody while the migration read as correct. Closed with
          `TestTenancyPolicies_ApplyToTheRightRoleAndCommand`, which pins policy name, command
          and role for all four tables, plus nine `has_table_privilege` assertions. That one
          test also killed **P1** (granting `refresh_tokens` to `app_tenant`) and **P3**
          (widening the `users` grant to full write) — both had been written off as later tasks'
          problems.
        - Round 2 (**17/20**): the harness itself was wrong. It counted any non-zero exit as a
          kill, so a Windows **Application Control block** — which never runs the test — was
          being credited as a mutant caught. Fixed to retry and report `BLOCKED`, and the fix
          immediately unmasked R3. A mutation harness that scores higher the worse the host
          behaves is measuring the host.
        - Round 3 (**18/21**): R4 added, killed.
      - **Two accepted survivors, both predicted in advance with a reason:**
        - **P2** — deleting the explicit `WITH CHECK` from a `FOR ALL` policy changes nothing,
          because PostgreSQL infers it from `USING`. That is exactly what design §The RLS policy
          template claims, so the survivor *confirms* the reasoning: it is written out for the
          day a policy is split per command, where the inference disappears.
        - **P4** — dropping `missing_ok` from `current_setting` survives, because every read in
          this task goes through `WithTenant`, which always sets the GUC. **T-01-015** owns the
          outside-`WithTenant` case and will kill it.
        - **R3 could not be run at all**: the mutated `dbtest` binary was refused by Application
          Control 8 times in a row. Its predicted verdict was survive, and nothing hangs on it:
          R4 covers the same field from the direction that matters. *(The conclusion first drawn
          from that 8-of-8 — "the block is deterministic per binary content" — was **wrong** and
          is corrected in T-01-014, which measured it properly. Third failed diagnosis of this
          block; see there.)*
      - **`app_public` has no policy on `shelters` in this phase, and no task creates one.**
        Design §Public surface lists `shelters` among the public tables, but the task board
        assigns public policies only to `pets` and `media` (T-01-019) with the suite at
        T-01-022. Rather than invent read access no test covers, this migration grants
        `app_public` nothing and `TestTenancyPolicies_...` asserts that. **Phase 06 needs
        shelter display data for the public catalog; the gap is real and belongs to whoever
        opens that phase.**
      - **Diagnosed a flake the harness had been carrying since 2026-08-29.** Whole packages
        were failing at 0.00s with `rootless Docker is not supported on Windows, failed to
        create Docker provider` — testcontainers' provider detection losing a race when several
        test binaries open Docker Desktop's named pipe at once. Measured: **6 of 17 parallel
        runs, 0 of 12 with `go test -p 1`**. `make test-api` now passes `-p 1`, and the comment
        in `dbtest/container.go` that guessed at container count as the cause has been corrected
        rather than left standing.
      - verify: `go vet` clean · `golangci-lint` **0 issues** · `govulncheck` 0 called
        vulnerabilities · `make test-api` green

- [x] **T-01-014** · `users` visibility and email uniqueness assertions  — **DONE 2026-08-30**
      - spec: data-model-core / *A user belongs to many shelters*, *Email comparison and
        uniqueness are case-insensitive by type*
      - tests: tenant A sees only users who hold a membership in A · differing case is the same
        user on INSERT · **a differently-cased lookup written as `WHERE email = $1` still finds
        the row** — the property `citext` buys over a `lower(email)` index
      - dod: run as `app_tenant`, never as the owner
      - est: 110
      - pilot: blacklisted (§7.2)
      - engram: —
      - **RESULT 2026-08-30.** `internal/db/rlstest/users_test.go`, plus `seedUser` and
        `seedMembership` generalised out of the T-01-013 helpers. `users` is the one table with
        no `shelter_id`, so it is the one table the A/B runner cannot express; this is its
        substitute.
      - **The visibility grid is 2×3, not the spec's two scenarios.** One direction and one
        tenant is not enough: `USING (false)` hides everything and passes "V is invisible to B",
        `USING (true)` shows everything and passes "U is visible to A". Only the full grid fails
        both — mutation confirmed it (U1 and U4 both die).
      - The third user is the one the spec does not name and the product needs most: **an
        adopter with no membership anywhere.** If `users` were readable by any authenticated
        tenant, every adopter's row would be visible to every shelter on the platform.
      - **`TestUsers_DifferingCaseIsTheSameUser` runs as the owner, and that is not a violation
        of the DoD but its consequence**: `app_tenant` holds no `INSERT` on `users` at all this
        phase, so no tenant-side write path exists to test. Phase 02's registration flow is what
        makes it runnable as an application role. Every *read* assertion goes through
        `WithTenant` on `TenantPool`.
      - The spec's third scenario — *an applicant is visible to the shelter they applied to* —
        needs `adoption_applications` (00009, T-01-027), where D6 adds a second permissive
        policy. `TestUsers_GetTheApplicantPolicyWhenAdoptionApplicationsLands` asserts the
        equivalence in both directions, so **T-01-027's RED is built here.** Third use of the
        deferral-with-a-guard shape (sqlc in T-01-012, the media FK in T-01-013).
      - **VERIFIED BY MUTATION, 7 mutants against the migration, 6 killed — and the survivor is
        the most valuable result of the task.**
        - **U2 survived: deleting the tenant scope from the users policy changes nothing
          observable.** Not a weak test. A policy expression is evaluated as the querying role,
          so the `EXISTS ... FROM memberships` subquery is **itself filtered by memberships' own
          policy** — the same mechanism behind PostgreSQL's familiar *"infinite recursion
          detected in policy for relation"*. The scope is applied twice.
        - **Confirmed, not inferred.** Re-ran U2 together with a permissive `SELECT` policy on
          `memberships`, and the grid failed exactly where it should: *a member of another
          shelter is invisible* → `visible = true, want false`.
        - Consequence, now written into the migration and the test: **`users` isolation is a
          CONJUNCTION, and half of it lives on another table.** A reader who notices the
          redundant clause and removes it leaves this table resting entirely on a rule nowhere
          near the file. The clause stays.
      - **Corrects T-01-013:** the Application Control block is **not** deterministic per binary
        content — that conclusion came from a single 8-of-8 sample. Measured properly on the
        `rlstest` binary: **5 of 5 blocked from the system temp, 3 of 5 from the in-repo
        `.gotmp`.** So the output directory matters and the binary matters, neither is decisive,
        and `make test-api`'s `GOTMPDIR` helps without fixing it. **Third failed diagnosis of
        this block.** It stays a host-policy exclusion the user owns; the project should stop
        theorising about it.
      - verify: `go vet` clean · `golangci-lint` **0 issues** · `govulncheck` 0 called
        vulnerabilities · `make test-api` green

- [x] **T-01-015** · Transaction-local scope and default-deny assertions  — **DONE 2026-08-30**
      - spec: tenant-isolation / *Tenant scope is transaction-local*, *Default-deny for
        unclassified access*
      - tests: after a `WithTenant` transaction commits **or** rolls back, `app.shelter_id` is no
        longer set on that connection · a query issued outside `WithTenant` sees **zero rows**,
        never the wrong rows · `refresh_tokens` refuses `app_tenant` entirely
      - dod: the commit and rollback paths are asserted separately — a leak on only one of them
        is the realistic bug
      - est: 100
      - pilot: blacklisted (§7.2)
      - **RESULT 2026-08-30. This task found a real defect in migration `00002` and fixed it.**
      - **THE BUG.** `current_setting('app.shelter_id', true)` does **not** return NULL on a
        connection that has already carried a scope. A GUC set inside a transaction and reverted
        at COMMIT or ROLLBACK does not go back to *unset* — it goes back to the **empty string**.
        So the policy expression became `shelter_id = ''::uuid` and raised **22P02** instead of
        returning zero rows. On a virgin connection it failed closed; on a reused one it failed
        loudly — and after the first request, **every pooled connection is a reused one**.
      - Nothing was ever exposed: the failure is loud, not leaky. What was broken is that the
        fail-closed contract the spec writes (*"MUST see zero rows"*) held or did not hold
        depending on whether that particular connection had served an earlier request. A
        contract with that shape is not a contract.
      - **Fix: `nullif(current_setting('app.shelter_id', true), '')::uuid` in all five policy
        expressions.** Both halves are load-bearing — `missing_ok` covers never-set, `nullif`
        covers reverted-to-empty — and mutants S1 and S2/S3 kill each half separately.
      - **This is a deviation from design §The RLS policy template**, which writes the bare
        `current_setting(...)::uuid`. Every later migration copies that template, so the design
        note is now wrong for `00003` onward. **Flagged for T-01-016 (Judgment Day), which
        reviews this template once for the whole phase.**
      - Scope teardown is asserted on a real pooled connection, not on `WithTenant`'s unit-test
        stub: the connection is **acquired and held**, so the read afterwards is the same
        session. Commit and rollback are separate cases per the DoD. A second test releases the
        connection and takes it back from a `MaxConns = 1` pool and compares backend PIDs —
        that is the production shape, and `WithTenant`'s own doc comment names it as the
        catastrophic case.
      - Default-deny is proven twice: against `refresh_tokens` (no policy, no grant — read and
        write, both under a **valid** scope so the refusal cannot be the scope's doing), and
        against a purpose-built probe table that has RLS and a correct policy and **no grant**.
        Both must fail with `42501`, not return an empty result: a missing grant that merely
        filtered would be indistinguishable from correct isolation.
      - **VERIFIED BY MUTATION, three rounds, 7 mutants, 7/7 killed — and rounds 1 and 2 each
        found a defect in the test itself.**
        - Round 1 (6/7): **S1 killed, which closes T-01-013's accepted survivor P4.** **S2
          survived** — removing the `nullif` from *memberships* went unnoticed while removing it
          from *shelters* did not, because the test named one table. Fixed by deriving the table
          list from `rlstest.Schema.Tenant` minus `Pending`, so a migration that adds a tenant
          table brings its case with it instead of waiting for someone to remember.
        - Round 2 (6/7): the survivor **rotated to S1**, which exposed a second defect. The
          "connection that never carried a scope" case was only genuinely virgin for the FIRST
          table in the loop; by the time `shelters` ran, the shared single-connection pool had
          already been through `memberships`. Fixed with a fresh pool per table.
        - Round 3: **7/7.**
        - A rotating survivor is worth more than a stable one. Both defects were in the test,
          both were invisible to a green run, and each was only visible because the mutant that
          should have died did not.
      - The per-table loop reports `fail-closed verified for N of M landed tenant tables` and
        **fails** any table holding zero rows: with no rows the policy expression is never
        evaluated and the case would pass vacuously.
      - **Application Control, fourth data point and a better one.** The block finally produced
        an informative message: *"the file contains a virus or potentially unwanted software"* —
        it is **Windows Defender quarantining the freshly linked test binary**, not an opaque
        policy. It blocked `rlstest.test.exe` 6 times in a row from both temp locations, so the
        whole task was run by compiling with `go test -c` into the scratchpad and executing the
        binary directly. **The actionable fix is a Defender exclusion**, which is the user's to
        make; the project should stop working around it.
      - verify: `go vet` clean · `golangci-lint` **0 issues** · `govulncheck` 0 called
        vulnerabilities · `make test-api` green
      - engram: —
      - note: the unit half is already covered against a stub in `tenant_test.go`; this is the
        half that needs a real pooled connection to be meaningful.

- [x] **T-01-016** · 🔴 **Judgment Day** — adversarial review before the RLS policies merge
      - spec: — (review gate, authors no code)
      - scope: the RLS policy template, the role model and grants of `00001`, `WithTenant` and
        `WithPublic`, the role guard, and the catalog meta-test
      - dod: two blind judges, at most two scoped fix rounds, verdict recorded in
        `docs/vault/40-bitacora/`
      - est: 0 (review, authors no lines)
      - pilot: blacklisted (§7.2)
      - engram: —
      - note: placed at the end of 01A because every later slice replays this template verbatim.
        Reviewing it four times is not a reserve. The master plan grants Judgment Day to exactly
        three merges in the whole project; this is one of them.
      - **RESULT 2026-08-30 — `JUDGMENT: APPROVED`.** Verdict in
        `docs/vault/40-bitacora/2026-08-30-judgment-day-rls.md`. Target
        `c5d3cf1b…`, 1199 lines across 6 files; fix delta `a217d24b…`. One fix round of two used.
      - Two blind judges in parallel, same frozen target. **Neither reported a CRITICAL and they
        agreed on nothing**, so the contract's "fix only what both confirm" rule authorised no
        correction at all. **Applying it mechanically would have merged a PII leak.**
      - **A1 — CONFIRMED and FIXED.** `member_visible_users` did not filter on
        `memberships.status`. Since `app_tenant` holds INSERT and UPDATE on `memberships`, a
        tenant could **mint read access to any user's row** by inserting an `invited` membership
        for that user id — and revoking it did not take the visibility away. Proven live on
        PostgreSQL 17: invisible → insert `invited` as `app_tenant` → visible → set `revoked` →
        **still visible**. Not a cross-tenant leak: a self-service read grant over PII.
        Fixed with `AND m.status = 'active'`, which is the word the spec already used
        (*"user U has an ACTIVE membership"*) and the DDL did not implement. Re-verified with
        the same attack: `invited` → invisible, `active` → visible, `revoked` → invisible.
      - **B1 — CONFIRMED and DEFERRED by the user.** Table-wide grants let `app_tenant`, on its
        own row: `UPDATE shelters SET status = 'verified'` (**LT-2 bypass — a shelter verifies
        itself**), raise `storage_quota_bytes`, and `UPDATE memberships SET role = 'owner'`. All
        three reproduced live. The fix is column-level grants, and which columns a tenant may
        write depends on endpoints Phase 03 has not written. → **Phase 02 (RBAC) and Phase 03**.
      - **B2 — CONFIRMED and DEFERRED by the user.** `CheckProtection` counts `pg_policy` rows
        without checking `polroles` or a tautological qual, so a future migration binding a
        policy to the wrong role or writing `USING (true)` still passes as protected. Same gap
        T-01-013's mutant M3 found; closed there only for `00002`'s four tables. → **T-01-017**.
      - **A2 — REFUTED.** The judge suspected the role guard was not wired in and said it could
        not confirm from its slice. Evidence outside that slice refutes it: `guardRole` is called
        from `newEnv` (`dbtest/container.go:308`) and `roles_test.go:169` observes it through
        `Env.GuardedRoles()`.
      - **The method lesson, recorded because it will recur.** A blind read-only judge cannot
        execute, so its findings arrive as `inferential` — both judges said so in as many words.
        That is not weak evidence, it is **missing capability**, and the orchestrator has it. A
        thirty-line probe turned two warnings into confirmed defects and one into a refutation.
        **Executable proof is stronger than a second judge** and satisfies the corroboration
        rule's purpose: what the rule forbids is acting on an opinion, not on a repro.
      - **Design deviation this gate existed to catch:** T-01-015's
        `nullif(current_setting('app.shelter_id', true), '')::uuid` differs from design §The RLS
        policy template, which writes the bare form. Both judges verified the deviation as
        correct. **The design note is now stale for `00003` onward and must be corrected there.**
      - Scoped re-judgment: both judges clean. They confirmed the fix closes **both halves**,
        that the `seedMembership` → `seedMembershipWithStatus` split is behaviour-preserving for
        all four call sites, and that the two new grid cases are **not vacuous** — both
        memberships target `ShelterA`, so removing the predicate flips them to `visible = true`.
      - `skill_resolution: mixed` — `jd-fix-a1` and `jd-b-round2` took the injected paths;
        `jd-a-round2` reported `fallback-path` and reviewed by direct inspection. Recorded as it
        happened.
      - **This APPROVED issues no receipt and carries no delivery authority.** It satisfies no
        commit, push, PR or release gate.
      - verify: `go vet` clean · `golangci-lint` **0 issues** · `govulncheck` 0 called
        vulnerabilities · `make test-api` green

- [x] **T-01-017** · Migration `00003_media.sql`  — **DONE 2026-08-30**
      - spec: data-model-core / *Media rows are tenant-scoped and logically deleted*
      - **INHERITED FROM T-01-016 (finding B2, confirmed):** `CheckProtection` in
        `rlstest/catalog.go` counts `pg_policy` rows without checking `polroles` or whether the
        qual is tautological, so a policy bound to the wrong role — or `USING (true)` — passes as
        "protected". `media` is the first table to cross that gap.
        `TestTenancyPolicies_ApplyToTheRightRoleAndCommand` pins role and command for `00002`'s
        four tables only; either widen it to `media` or lift the check into the meta-test.
      - **ALSO INHERITED:** `TestShelters_MediaColumnsGetTheirForeignKeyWhenMediaLands` goes RED
        the moment this migration lands. `shelters.logo_media_id` and `cover_media_id` need their
        foreign key **here**, and the design deviation from T-01-015
        (`nullif(current_setting(...), '')`) must be copied into this migration's policy — the
        design's own template is stale.
      - RED first: add `media` to the A/B suite and the meta-test tenant list.
      - build: `media` with `shelter_id`, nullable `deleted_at`, **tenant policy only**, grants ·
        `UNIQUE (id, shelter_id)` — `documents` (T-01-031) needs it for its composite FK, and
        every composite-FK parent declares that key in its own migration
      - dod: A/B green for `media` · `UNIQUE (id, shelter_id)` present · **no** `app_public`
        policy in this migration
      - est: 72
      - pilot: blacklisted (§7.2)
      - engram: —
      - parallel: T-01-018 (independent tables; serialise edits to the shared A/B suite file)
      - note: **RESOLVED 2026-08-29.** Design §Public surface put `media`'s public policy here,
        but it depends on `pets` (`00005`), which does not exist at `00003`. The tenant policy
        lands here, the **public policy lands in `00005` (T-01-019)**. A migration may add a
        policy to an earlier migration's table; it may not reference a table that does not exist.
      - **RESULT 2026-08-30.** `00003_media.sql`, the `media` A/B case, and
        `internal/db/rlstest/media_test.go`. The `Pending` ledger moved to **5 of 19 verified**.
      - **The `shelters` → `media` keys are COMPOSITE, not single-column, and that is a
        correction to what T-01-013 promised.** T-01-013 deferred them as plain
        `REFERENCES media (id)`. Referential integrity checks always bypass row security, so
        that spelling would let shelter B set its logo to a media row owned by shelter A — a row
        B cannot see, cannot read, and would still be publishing.
        `FOREIGN KEY (logo_media_id, id) REFERENCES media (id, shelter_id)` makes it
        unrepresentable instead. The pair is `(logo_media_id, id)` because for `shelters` the
        tenant column IS `id`, and MATCH SIMPLE means a NULL logo is checked against nothing.
        **This is also the first user of `media`'s `UNIQUE (id, shelter_id)`** — the task text
        attributed that key to `documents` (T-01-031) alone; it is needed today.
      - `TestShelters_CannotClaimAnotherTenantsMedia` proves the difference, because **both
        spellings compile and both migrate** and only one is correct. It carries its own
        anti-vacuity step: tenant A must be able to point at its OWN media row first, or a
        constraint refusing every value would pass.
      - **INHERITED B2 from T-01-016, discharged.** `TestTenancyPolicies_ApplyToTheRightRoleAndCommand`
        was narrow to `00002`'s four tables, so `media` would have crossed the meta-test's
        role-blindness uncovered. Widened to five tables and eleven privilege assertions,
        including **`app_public` must NOT hold SELECT on `media`** — the public policy belongs to
        `00005`, and listing the whole inventory means adding it early fails here. From now on
        **every migration adds its rows to both tables in that test.**
      - **INHERITED design deviation, discharged.** `design.md` §The RLS policy template still
        showed the bare `current_setting(...)::uuid`, which T-01-015 proved wrong on a reused
        pooled connection. Corrected in place, with the reason, plus the D6 example, which also
        gained the `AND m.status = 'active'` that T-01-016 added. Ten migrations copy that
        template; leaving it stale would have propagated both defects.
      - **VERIFIED BY MUTATION, two rounds, 12 mutants, 12/12 killed.**
        - Round 1 (11/12): **M12 survived** — dropping the `kind` CHECK broke nothing. §4.2 makes
          `kind` a closed union and Phase 04's derivation pipeline branches on it, so a third
          value would reach a switch with no arm. Closed with `TestMedia_KindIsAClosedUnion`,
          which asserts both declared members are accepted before asserting a third is refused.
        - Round 2: **M11 rotated to SURVIVED** — deleting the `Down`'s
          `DROP CONSTRAINT` broke nothing. **That was a harness defect, not a coverage gap**: the
          round-trip lives in `./internal/db/` and the harness only ever ran
          `./internal/db/rlstest/`. Re-run against the right package it dies loudly with
          `2BP01: cannot drop table media because other objects depend on it`, in three tests.
          **Third time the mutation harness itself has been the thing at fault** — after
          crediting Application Control blocks as kills (T-01-013) and sharing one connection
          across a per-table loop (T-01-015). A mutation round is an instrument and instruments
          need calibrating: **the target package is part of the mutant, not a setting.**
      - dod check: A/B green for `media` · `UNIQUE (id, shelter_id)` present and asserted
        directly · **no** `app_public` policy and **no** `app_public` grant, both asserted
      - verify: `go vet` clean · `golangci-lint` **0 issues** · `govulncheck` 0 called
        vulnerabilities · `make test-api` green

- [x] **T-01-018** · Migration `00004_reference_data.sql` — `species`, `breeds` + seeds  — **DONE 2026-08-30**
      - spec: pet-catalog-data / *Reference data is global and read-only*
      - build: `species`, `breeds`, seeds for both · RLS `ENABLE` + `FORCE` with
        `FOR SELECT USING (true)` · `SELECT` grant only, to both roles
      - tests: neither role can write · both roles read the same rows · seeds are idempotent on
        replay
      - dod: global reference data carries **no** `shelter_id` and is **not** in the tenant set
      - est: 170
      - pilot: blacklisted (§7.2)
      - engram: —
      - parallel: T-01-017
      - **RESULT 2026-08-30.** `00004_reference_data.sql` (3 species, 31 breeds) and
        `internal/db/rlstest/reference_test.go`. Ledger now **7 of 19 verified**.
      - **The spec's own idempotency scenario cannot fail, and that is worth saying out loud.**
        *"Applied, reversed and applied again, each holds exactly the seeded number of rows"* —
        `down-to 0` DROPS both tables, so the second apply always seeds a clean slate no matter
        how the INSERT is written. The round-trip suite satisfies it already and it proves
        nothing. The failure that exists is replaying the seed against a database that **already
        holds the rows**: a second deployment path, a hand-run migration, a restored dump.
        `TestReferenceData_SeedsAreIdempotentOnReplay` reads the statements between
        `-- seeds:begin` / `-- seeds:end` **out of the migration file** and executes them a
        second time against the live schema, so it exercises the real SQL rather than a copy
        that drifts. It fails loudly if the markers go missing — a test that silently found
        nothing to replay would be the same species of lie as the scenario it replaces.
      - `ON CONFLICT` targets the **natural** key (`code`, and `(species_id, name)`), not the
        primary key. With fixed seed identifiers both spellings are a no-op today; the natural
        key is what makes a SECOND seed, written later with a different id for the same breed,
        collide instead of pass.
      - RLS is `ENABLE` + `FORCE` here too, even though there is nothing to isolate. A policy on
        a table whose row security is off is inert, and **"global" has to mean "readable by both
        roles", never "unprotected"**. Read-only is two independent layers: a `FOR SELECT` policy
        and no other, and a `SELECT` grant and no other. Either alone refuses a write; both are
        stated so losing one is not enough to open the table. Asserted across **12 subtests** —
        three commands × two tables × two roles.
      - `USING (true)` is not the tautology it would be on a tenant table: it says every row, to
        both roles, for SELECT and nothing else.
      - **Both roles must see the SAME rows**, and that is not implied by either being able to
        read. Asserted against the owner's count, from two different tenant scopes and from
        `WithPublic` — reference data that varied by tenant would be the exact bug of scoping it.
      - `app_public`'s write attempts may be refused by the READ ONLY transaction (`25006`)
        before the grant (`42501`) is ever consulted. Both are accepted: the point is that the
        refusal comes from the access layer rather than from data.
      - Seed content is a domain decision, not filler: **`Mestizo` is first for both species**
        because most animals a shelter takes in have no known breed, and a catalog whose breed
        filter cannot express "mixed" hides the majority of its own rows. `code` is English (an
        identifier code branches on); `name` is es-MX (display text, §11.1), and when a second
        locale arrives `code` is the join key a translation table hangs off.
      - Identifiers are **literal, not generated**. D3 governs application identifiers; a seed is
        not one. Fixing them means `pets.species_id` resolves to the same row in every
        environment, which is what makes a fixture, a test and a client mapping portable.
      - **VERIFIED BY MUTATION, 12 mutants, 11 killed.** Every mutant now names the package that
        **observes** it — the correction T-01-017 forced — so R11 (the `Down`'s drop order) ran
        against `./internal/db/`, where the round-trip lives, and died there.
      - **One accepted survivor, predicted:** R1, retargeting `ON CONFLICT` from the natural key
        to the primary key. With fixed seed ids both are a no-op, so nothing observable changes
        today. It is not a correctness hole either: a future seed reusing a name with a new id
        would hit `UNIQUE (species_id, name)` and raise `23505` rather than duplicate — the
        difference is a loud failure instead of a quiet skip, and R3 already proves that
        constraint is present.
      - dod check: **no** `shelter_id` on either table, asserted directly · neither is in the
        tenant set, asserted against `Schema.Tenant` itself
      - verify: `go vet` clean · `golangci-lint` **0 issues** · `make test-api` green

- [x] **T-01-019** · Migration `00005_pets.sql` — the catalog core  — **DONE 2026-08-30**
      - spec: pet-catalog-data / *Pets are tenant-scoped*, *The public catalog shows only
        published, available, non-deleted pets*
      - build: `pets` with every §4.3 column · `UNIQUE (id, shelter_id)` for the child composite
        FKs · the §4.6 indexes · `tenant_isolation` + `public_catalog` policies ·
        **the `media` public policy deferred from T-01-017**
      - dod: A/B green for `pets` · the deferred `media` public policy lands **here**, not
        re-deferred · `app_public` sees a media row only through a published, non-deleted,
        available pet
      - est: 239
      - pilot: blacklisted (§7.2)
      - engram: —
      - **RESULT 2026-08-30. One DoD item could not be met, and the reason is a scheduling
        error the board has now made twice.**
      - **`media`'s public policy is NOT here, and it cannot be.** Design §Public surface put it
        in `00003`; T-01-017 moved it to `00005` on the grounds that it depends on `pets`. **It
        does not.** The spec's own scenario is *"media M is **attached** to a draft pet"*, and
        attachment is `pet_media` — migration `00006`, T-01-020. A policy cannot reference a
        table that does not exist. It lands there, with the table that makes the join
        expressible, and `TestMedia_GetsItsPublicPolicyWhenPetMediaLands` asserts the
        equivalence in both directions so a third re-deferral cannot happen by being forgotten.
        Until then `media` has no `app_public` policy **and** no `app_public` grant, which the
        same test pins: **a grant without its policy is the widest possible state, not the
        safest.**
      - Everything else in the DoD is met: A/B green for `pets`, and the public-catalog
        behaviour is asserted directly.
      - **The public catalog's three conditions are three separate cases, not one.** The
        realistic bug is a predicate that drops ONE of them, and a case that flips all three at
        once cannot say which. Each is retracted on its own, after asserting the pet WAS public
        first.
      - **The default state is asserted as hard as the transitions**: a new pet is `draft` with
        a null `published_at`, so **forgetting to publish keeps it private**. A default of
        `available` would make the safe path the one nobody takes, and mutation confirmed
        nothing else catches it.
      - **`UNIQUE (shelter_id, microchip_id)`, deliberately NOT global.** A microchip number is
        globally unique in the world, so a global constraint is the "correct" modelling and it
        is a cross-tenant leak. Uniqueness is checked **before any policy** (proven for the
        forged-insert path in T-01-013), so `UNIQUE (microchip_id)` turns every INSERT into an
        **existence oracle**: type a chip number, read the 23505, learn that another shelter
        holds that animal. Cross-shelter duplicate detection belongs to a moderation view under
        the owner role, not to a constraint every tenant can probe.
      - Nullable `sterilized` / `vaccinated` / `dewormed`: NULL is "not known", which for an
        animal that arrived last night is the truth. `NOT NULL DEFAULT false` would record "not
        vaccinated" for every intake and no adopter could tell the two apart.
      - **VERIFIED BY MUTATION, two rounds, 16 mutants, 16/16 killed.**
        - Round 1 (14/16): both survivors were predicted, and both were closed rather than
          accepted. **P12** — dropping `UNIQUE (id, shelter_id)` broke nothing, the same blind
          spot `media` had; `TestCompositeTenantKeys_AreDeclaredByTheirParents` now covers both
          parents, and every future composite-FK parent adds a row. **P13** — retargeting the
          microchip key from per-shelter to global broke nothing, so the security property
          argued for in the migration's own comment was unasserted;
          `TestPets_MicrochipUniquenessIsPerShelterNotGlobal` asserts both halves.
        - Round 2: **16/16.**
      - **A test from T-01-011 broke, and the fix generalises.** `TestCheckCatalogAt_...` created
        a probe table literally named `pets` — chosen back then because `pets` was declared and
        not yet created. This migration created it and the test failed with `42P07`. Renaming to
        another literal would only postpone the identical failure **eleven more times**, once per
        remaining pending table. The probe now takes its name from `Schema.Pending` at run time,
        so it survives every future migration and skips loudly, with a reason, only when the
        ledger empties at the end of the phase.
      - **Discovered gap, flagged not fixed:** `pets.breed_id` has a single-column FK to
        `breeds (id)`, so `species_id = cat` with `breed_id = a dog breed` is representable. The
        database-as-last-line-of-defence rule says it should be a composite FK to
        `breeds (id, species_id)`, which needs `UNIQUE (id, species_id)` on `breeds`, declared in
        `00004` — and reopening a closed migration is outside this task's build list. Unlike the
        composite TENANT key it is not a security boundary: it corrupts the adopter filter
        (AD-3), not isolation. **Recorded in FASE-01.md's open questions.**
      - verify: `go vet` clean · `golangci-lint` **0 issues** · `govulncheck` 0 called
        vulnerabilities · `make test-api` green · ledger **8 of 19 verified**

- [x] **T-01-020** · Migration `00006_pet_children.sql` — `pet_media`, `pet_health_records`, `pet_status_history`  — **DONE 2026-08-31**
      - spec: pet-catalog-data / *Child rows cannot cross tenants*
      - **INHERITED FROM T-01-019, and this is its THIRD scheduling:** `media`'s `app_public`
        policy lands **here**. Design §Public surface put it in `00003`; T-01-017 moved it to
        `00005` believing it depended on `pets`; it depends on **`pet_media`**, which this
        migration creates. `TestMedia_GetsItsPublicPolicyWhenPetMediaLands` goes RED the moment
        `pet_media` exists without it. The policy must scope `media` through a pet that is itself
        public, and the nested `pets` lookup is filtered by `public_catalog` already, which
        reinforces rather than widens it. `app_public` also needs its `SELECT` grant on `media`,
        withheld today on purpose.
      - build: all three with a denormalised `shelter_id` and a **composite FK** to
        `pets (id, shelter_id)` (D5) · `pet_media` PK is the pair `(pet_id, media_id)`, not a new
        uuid — §4.3 gives it no `id` and a pure join row needs no separate identity
      - tests: A/B for all three
      - dod: every child carries the composite FK, not a single-column one
      - est: 171
      - pilot: blacklisted (§7.2)
      - engram: **8 candidatos aprobados por el usuario 2026-08-31.** `mascotapp/ops/windows-appcontrol-go-tests` obs-09d0eb05887b634f (reemplaza los dos registros con la causa falsa) · `mascotapp/domain/append-only-tables` obs-99c2e3298b483ab8 · `mascotapp/security/rls-truncate` obs-7547cb4172cbc931 · `mascotapp/security/rls-exists-grant` obs-e2fdbf0b785f08df · `mascotapp/arch/public-catalog` obs-db33d8e90bb53b65 · `mascotapp/convention/testing-aborted-transaction` obs-8ab9e31b8abb632d · `mascotapp/convention/testing-catalog-assertions` obs-ce88a1825e176a88 · `mascotapp/convention/testing-conformance-flags` obs-64c7b74b30e1800a
      - open-q: **whether `pet_status_history` should be append-only.** §4 does not say. It ships
        as an ordinary tenant table unless the user says otherwise; locking it down is cheap now
        and expensive later. Do not answer silently.
      - **RESULT 2026-08-31. The open question was answered by the USER, and the plan had
        already answered half of it.** The board said "§4 does not say", which is true of §4 and
        false of the plan: §1.1's LT-5 mitigation reads *"Estados de `pets` +
        `pet_status_history` **inmutable desde el día 1**"*, and `FASE-05.md` already carries it
        forward. Asked with that evidence, the user chose **append-only, with the foreign key
        that makes it real**.
      - **`ON DELETE RESTRICT`, not `CASCADE`, and the design's own example was wrong.** D5
        writes `ON DELETE CASCADE` on this exact key. `app_tenant` holds `DELETE` on `pets`, so
        a cascading key lets a shelter erase an animal's entire trail **by deleting the animal**
        — never touching the protected table, never tripping the trigger, straight through the
        one door nobody was watching. **Append-only whose parent cascades is theatre.** It is
        not a workaround either: LT-5's rule is that a pet leaves by CHANGING STATUS, so a pet
        with recorded history is a pet that can only be soft-deleted, which is what the product
        wanted anyway. `pet_media` and `pet_health_records` keep `CASCADE`; they are records,
        not history.
      - **Four layers, each stopping a different actor.** Per-command `tenant_read` /
        `tenant_append` policies (the ABSENCE of an UPDATE/DELETE policy is the second layer);
        `REVOKE UPDATE, DELETE, TRUNCATE ... FROM app_tenant, PUBLIC`; a `BEFORE UPDATE OR
        DELETE` trigger; and a **separate `BEFORE TRUNCATE` statement trigger**, because
        TRUNCATE is the one write no row-level policy can ever see — it removes every row
        without visiting any, so `USING` and `WITH CHECK` are never consulted.
      - The trigger is not redundant with the grant. Neither the grant nor the missing policy
        survives a role with `BYPASSRLS`, and **on Neon that is not hypothetical**:
        `neon_superuser` carries it. RLS is bypassed by such roles; **triggers are not.** The
        test therefore runs as the OWNER — a superuser holding every privilege and skipping
        every policy — so a pass can only be the trigger's doing. Its SQLSTATE is plpgsql's
        default `P0001`, deliberately distinct from the grant's `42501` so the two layers stay
        distinguishable.
      - **The A/B runner was extended rather than exempted.** `AppendOnly` swaps the write
        probes for STRICTER ones — "the statement was REFUSED" instead of "reached no rows".
        Skipping them was the easy route and the wrong one: a table whose grants were quietly
        restored would then pass by not being looked at. It generalises to `application_events`
        (T-01-029) and `audit_log` (T-01-032).
      - **`media`'s public policy landed, on its third scheduling and its first correct one**,
        and it works by COMPOSITION rather than duplication: `media`'s policy says only "there
        is an attachment", `pet_media`'s says only "there is a pet", and each subquery is
        itself filtered by the referenced table's RLS under `app_public`. So "a public pet" is
        defined in exactly ONE place — `public_catalog` on `pets` — and a photo follows its pet
        automatically. Proven by walking a pet through draft → published → retracted and
        watching the photo follow, with the media row never touched.
      - **`UNIQUE (shelter_id, pet_id) WHERE is_primary`, deliberately not keyed on `pet_id`
        alone** — the same existence oracle as `pets.microchip_id` (T-01-019). Uniqueness is
        checked before any policy, so a global key lets tenant B name A's pet and read the
        answer off the SQLSTATE: `23505` means that pet already has a cover photo, `23503` means
        it does not exist. Two different answers to a question B has no right to ask.
      - **`pet_health_records.document_media_id` gets its own composite key.** It is the second
        reference on the same row and the easy one to miss — the key to `pets` is the one the
        design writes out, so a single-column `REFERENCES media (id)` would look finished while
        letting a tenant link another shelter's document.
      - **VERIFIED BY MUTATION AT TWO LEVELS. 16/16 in SQL, 10/10 in Go, both first round.**
        The SQL round ran first because the Go suite could not run at all (see below); it killed
        14/16 on its first pass and both survivors were closed rather than accepted. **M15** —
        widening `pet_media`'s `WITH CHECK` to `true` broke nothing, because the probe set only
        tested the FK path: a composite key stops a child pointing at ANOTHER tenant's parent
        and does nothing about a child stamped with another tenant's `shelter_id` whose parent
        is that same tenant's. **M13** — dropping `app_public`'s grant on `pet_media` aborted
        the probe run and was scored "not evaluated" instead of the kill it was, the same
        instrument-versus-net error T-01-012 found.
      - **M13 settled a question this phase had been assuming rather than proving:** a policy's
        `EXISTS` subquery **does** require the querying role to hold `SELECT` on the referenced
        table. Without the grant on `pet_media`, `app_public` does not get zero media rows — it
        gets a permission error. WRITE access to a bridge table is READ access through it, and
        now so is the absence of read access.
      - **THE TEST SUITE COULD NOT RUN ON THE HOST, AND THE CAUSE THIS PROJECT RECORDED THREE
        TIMES WAS WRONG.** It is **Windows Smart App Control**, not Defender:
        `Get-MpComputerStatus` reports `AMRunningMode: Passive Mode` with real-time protection
        OFF, while `VerifiedAndReputablePolicyState` is `1` (enforcement). **The Defender
        exclusion this project kept recommending would have done nothing.** Smart App Control
        blocks unsigned executables with no reputation and, unlike Defender, **has no exclusion
        list at all** — it is on or off, and turning it off cannot be undone without
        reinstalling Windows. Every `go test` links a fresh unsigned binary, so there is no
        in-repo workaround; `golangci-lint`, `govulncheck` and `go vet` keep working because
        they already have reputation, which is why linting never showed the symptom.
      - **Fixed by `make test-api-container`**, which runs the same suite on Linux. Two mounts
        are load-bearing: the Docker socket, because testcontainers starts Postgres as a
        SIBLING container (hence `TESTCONTAINERS_HOST_OVERRIDE=host.docker.internal`), and the
        host's `GOMODCACHE` with `GOPROXY=off`, because the network intercepts TLS and module
        downloads fail with `certificate signed by unknown authority`.
      - **The container run immediately found two defects in the new test code**, neither of
        which the SQL round could see. (1) On an append-only table the first refusal **aborts
        the transaction**, so the second probe could only ever report `25P02` and its property
        was unasserted; each probe now runs under its own savepoint. (2) `hasPartialUniqueOn`
        was unfalsifiable: casting `indkey` to `int2[]` yields an array with **lower bound 0**
        while `array_agg` builds one based at 1, and PostgreSQL compares bounds as well as
        elements, so the two never matched however right the index was.
      - **A test from T-00-019 broke, and it was right to.**
        `TestDevcontainerInstallsEveryToolTheMakefileInvokes` read every tab-indented PHYSICAL
        line as its own command, so the first backslash-continued recipe this Makefile has ever
        had became commands named `docker`, `-v` and `golang:1.27`. Fixed by joining
        continuations — and because that LOOSENS a check, it was mutated: a fake tool planted
        inside a continued recipe is still caught by name.
      - verify: `go vet` clean · `golangci-lint` **0 issues** · `govulncheck` 0 called
        vulnerabilities · `make test-api-container` green across all five packages · ledger
        **11 of 19 verified**

- [x] **T-01-021** · **Child-table orphan test** — the assertion that proves D5  — **DONE 2026-08-31**
      - spec: tenant-isolation / *A child row cannot reference another tenant's parent*
      - tests: as tenant B, inserting a child row referencing tenant A's parent id fails with
        SQLSTATE **`23503`** · a nonexistent parent id fails with the **same** error, which is
        what closes the cross-tenant existence oracle
      - dod: a `42501` here means the policy caught it and **the FK is not being exercised** —
        that is a failing test, not a passing one
      - est: 105
      - pilot: blacklisted (§7.2)
      - engram: `mascotapp/security/error-oracles` obs-f653583f24128395 · `mascotapp/convention/testing-shared-fixtures` obs-3acb038bf64919cf (aprobados por el usuario 2026-08-31)
      - note: PostgreSQL documents that referential integrity checks **always bypass row
        security**. Without this test the composite FK can be dropped in a refactor and no
        read-path test notices.
      - **INHERITED FROM T-01-020 — what is already proven and what is still open.** The A/B
        suite proves the *other* cross-tenant write: a child stamped with tenant A's
        `shelter_id`, refused by `WITH CHECK` with `42501`.
        `TestPetHealthRecords_CannotAttachAnotherTenantsDocument` proves the FK path for the
        document reference. **Still unasserted, and it is this task's whole point: that a
        FOREIGN parent id and a NONEXISTENT parent id come back with the SAME error.** Only
        that equality closes the oracle; two distinguishable failures are an answer to a
        question the tenant has no right to ask, and nothing written so far compares them.
      - **RESULT 2026-08-31.** `internal/db/rlstest/child_orphan_test.go`.
        `TestChildTables_CannotReferenceAnotherTenantsPet` is table-driven over ALL THREE
        children rather than over `pet_media` alone — D5 applies to each, and a key dropped
        from one of them in a refactor has to fail somewhere. Each case runs three inserts as
        tenant B: its OWN pet (anti-vacuity), tenant A's pet, and a pet that does not exist.
      - **The row carries B's OWN `shelter_id`, and that is the entire design of the test.**
        The A/B suite already covers the other cross-tenant write — a child stamped with A's
        `shelter_id`, refused by `WITH CHECK` with `42501`. Stamping B satisfies the policy so
        it steps aside and the FOREIGN KEY is what answers. The DoD's rule is enforced as its
        own branch with its own message: a `42501` here fails the test loudly, because it
        means the policy got there first and the key could be dropped tomorrow unnoticed.
      - **Refusal is not the property; INDISTINGUISHABLE refusal is.** Two failures that
        differ let B enumerate A's pets one id at a time. The assertion compares SQLSTATE *and
        constraint name* between the foreign parent and the nonexistent one — the same oracle
        `pets.microchip_id` (T-01-019) and `pet_media`'s cover-photo key (T-01-020) were
        scoped per shelter to close.
      - `TestPetMedia_CannotReferenceAnotherTenantsMedia` extends it to **the second reference
        on the same row**, which is the one the design does not write out and therefore the
        one a refactor drops. It leaks about a different table: distinguishing "that photo is
        someone else's" from "that photo does not exist" is a probe over another shelter's
        storage.
      - **THE FULL SUITE FAILED, AND THE COUPLING IT EXPOSED WAS REAL.** This file sorts FIRST
        in the package, so it seeded before `TestTenantIsolation` for the first time, and
        broke it two different ways. (1) The `shelters` case has `TenantColumn: "id"` — tenant
        A's row IS shelter A — so that case **must be the one that creates it**; seeding it
        first turned its insert into a bare `duplicate key`. (2) The `pets` case probes an
        unqualified `DELETE FROM pets`, the only shape that can catch a permissive DELETE
        policy, and a leftover pet of B carrying status history refuses it outright through
        `ON DELETE RESTRICT` — the FK from T-01-020 doing exactly its job, in a case that has
        nothing to do with it.
      - **Fixed at the coupling, not at the symptom.** `freshTenant` mints this file's own
        shelters: the A/B fixtures belong to the A/B runner and an unrelated test has no
        business reaching for them. Then the hazard was made LOUD instead of documented — the
        `shelters` case gained a `Fixture` that checks whether shelter A already exists and
        names the cause, so the next occurrence is one line to read instead of a puzzle.
      - **MUTATION: 7/7, first round.** M1–M3 collapse each child's parent key to a
        single-column `REFERENCES pets (id)`; M4 does the same to `pet_media`'s media
        reference; all four are ACCEPTED writes without the composite key, which is the leak.
        M5/M6 mutate the equality clause itself — no schema edit can make a foreign parent
        answer differently from a nonexistent one while both stay refused, so that clause is a
        REGRESSION GUARD, and what can be proved is that its branch is live and the right
        message fires. M7 borrows `env.ShelterA` again and confirms the new guard catches it
        BY NAME. The harness scores a mutant killed only when a TEST reported `--- FAIL`: a
        non-zero exit alone is also a compile error, and crediting that against the net is the
        T-01-012 mistake.
      - verify: `go vet` clean · `golangci-lint` **0 issues** · `govulncheck` 0 called
        vulnerabilities · `make test-api-container` green across all five packages

- [x] **T-01-022** · `app_public` read-only catalog suite  — **DONE 2026-08-31**
      - spec: pet-catalog-data / *The public catalog shows only published, available,
        non-deleted pets*
      - tests: `app_public` sees only published + available + non-deleted pets · cannot write
        anything · sees a `media` row only through such a pet · sees no draft pet and no pet of
        an unverified shelter
      - dod: run through `WithPublic` on the separate pool and role
      - **INHERITED FROM T-01-020/021.** The public chain is already proven for MEDIA:
        `TestPublicMedia_IsReachableOnlyThroughAPublicPet` walks a pet through draft →
        published → retracted and watches the photo follow, and
        `TestPublicMedia_SoftDeletedPhotoLeavesTheCatalog` covers the one condition `media`
        owns rather than inherits. What is NOT covered and belongs here: `app_public` cannot
        WRITE anything, and it sees no pet of an UNVERIFIED shelter — the second one may not
        be expressible yet, since `app_public` has no policy on `shelters` at all (open
        question in FASE-01.md).
      - **Mint your own shelters (`freshTenant`), do not reuse `env.ShelterA`/`ShelterB`.**
        The `shelters` A/B case scopes by `id` and has to be the one that creates tenant A;
        its Fixture now fails by name if anything seeds it first.
      - est: 120
      - pilot: blacklisted (§7.2)
      - **RESULT 2026-08-31.** `internal/db/rlstest/public_catalog_test.go`. The read path was
        already covered by T-01-019/020 — the three conditions each retracted alone, the
        cross-shelter span, and the whole media chain — so this task is the WRITE half plus
        one finding.
      - **`app_public` cannot write, proven at two layers with two SQLSTATEs.** `WithPublic`
        opens its transaction `pgx.ReadOnly`, so a write is refused with `25006` before any
        policy or grant is consulted; on an ORDINARY transaction from the same pool the
        database refuses on its own with `42501`. Only the second is a property of the schema
        rather than of the client, which is why both are asserted separately.
      - **MUTATION CORRECTED A CLAIM THIS TASK HAD ALREADY WRITTEN DOWN.** The comment said
        the ordinary-transaction case pinned the GRANTS. It does not. M1 — `GRANT INSERT ON
        pets TO app_public` — left **both** behavioural cases green, because `public_catalog`
        is `FOR SELECT`, so with the grant in place the MISSING INSERT POLICY refuses instead,
        **with the same 42501 a missing grant reports**. Two different layers, one SQLSTATE,
        and no behavioural probe can separate them; matching on message text would, and is not
        worth the brittleness. The comment now says what the mutant showed.
      - **So the grant matrix is load-bearing, not belt-and-braces.**
        `TestPublicRole_HoldsNoWritePrivilegeOnAnyTable` enumerates EVERY table from the
        catalog × INSERT/UPDATE/DELETE/TRUNCATE. The hand-picked cells in the isolation
        inventory cover the tables somebody remembered on the day they wrote it; the public
        role's contract is "no writes ANYWHERE", so every table a later migration adds is
        covered the day it lands. TRUNCATE is in the matrix because a policy-shaped mental
        model misses it entirely (T-01-020).
      - **FINDING — LT-2 IS NOT ENFORCED BY THE DATABASE, and the suite was pinning the
        opposite.** §1.1 makes `pending_verification` a hard MVP requirement: *"un refugio no
        puede publicar hasta ser verificado manualmente"*. `shelters.status` defaults to
        `pending_verification`, so the column is there and nothing reads it. `public_catalog`
        on `pets` filters on `status`, `published_at` and `deleted_at` and on nothing else —
        so **every public-catalog test in this package publishes from a shelter that was never
        verified and asserts the pet IS visible.** Green was reading as "verification works".
      - It is NOT fixed here, and the reason is structural rather than scope discipline: the
        condition needs `EXISTS (SELECT 1 FROM shelters ...)`, and a policy's subquery requires
        the querying role to hold `SELECT` on the referenced table (T-01-020, mutant M13).
        `app_public` has neither grant nor policy on `shelters`, so adding the condition alone
        would not narrow the catalog — **it would break it with a permission error on every
        public read**, which M6 confirms by taking down ten other cases. That is a migration
        and it is the open question this phase already carries.
        `TestPublicCatalog_DoesNotYetEnforceShelterVerification` is a CHARACTERISATION test:
        it states today's behaviour, fails the day the condition lands with instructions to
        invert it, and asserts the two halves can only land TOGETHER.
      - **MUTATION: 6/6, first round**, run over the whole package recording WHICH tests died.
        M1 grant on pets · M2 TRUNCATE on `pet_status_history` · M3 UPDATE on `media` (a table
        no behavioural probe touches) · M4 `WithPublic` stops opening read-only · M5 forces the
        half-landed branch of the LT-2 guard · M6 lands the LT-2 condition without the grant.
      - verify: `go vet` clean · `golangci-lint` **0 issues** · `govulncheck` 0 called
        vulnerabilities · `make test-api-container` green across all five packages
      - engram: `mascotapp/domain/shelter-verification` obs-85415c0b8700bec6 · `mascotapp/convention/testing-layer-attribution` obs-fedf7a4ef4efc6e3 (aprobados por el usuario 2026-08-31)

- [x] **T-01-023** · Migration `00007_form_templates.sql` — templates and immutable versions  — **DONE 2026-08-31**
      - spec: dynamic-forms-data / *A published form version is immutable*
      - build: `form_templates`, `form_template_versions` with a denormalised `shelter_id` and a
        composite FK (D5) · `UNIQUE (shelter_id, key)` · `UNIQUE (template_id, version)` · a
        trigger refusing UPDATE/DELETE on a row whose `published_at` is not null
      - tests: A/B for both · editing a published version fails
      - dod: immutability is enforced by the database, not by application discipline
      - **RESULT 2026-08-31.** `migrations/00007_form_templates.sql` +
        `rlstest/form_templates_test.go` + the two A/B cases. Written test-first: the suite was
        run RED against the missing tables (`42P01`) before the migration existed.
      - **THE VERSION KEY IS SCOPED PER SHELTER, AND THAT DEVIATES FROM §4.4 ON PURPOSE.** The
        spec writes `UNIQUE (template_id, version)`. Probed against PG 17 rather than assumed:
        **the unique index is checked BEFORE the foreign key** (referential checks run as AFTER
        triggers; the index insert happens on the heap write). Proof, run directly in psql: a
        row that is both a duplicate AND names a foreign parent reports `23505`, while one that
        only names a foreign parent reports `23503`. So a global key is a **cross-tenant
        existence oracle** — tenant B, stamped with its OWN `shelter_id` so the policy steps
        aside, names A's template and reads A's form history off the SQLSTATE. Third time this
        phase after `microchip_id` (T-01-019) and the cover photo (T-01-020). Scoping to
        `(shelter_id, template_id, version)` preserves the guarantee EXACTLY inside a tenant —
        a template belongs to one shelter, so per-shelter uniqueness over its versions IS
        per-template uniqueness — and collapses the two answers into one for everybody else.
        Mutant **M2** restores the spec's literal key and dies, so this is evidence rather than
        preference.
      - **Immutability is CONDITIONAL, which is what makes it unlike `pet_status_history`.** A
        draft is fully editable and discardable; a published row is frozen. No grant can say
        "these rows but not those", so `UPDATE`/`DELETE` ARE granted and a trigger decides per
        row — which is also the only layer that survives a `BYPASSRLS` role, so the test runs
        as the OWNER.
      - **The trigger guards `published_at` too, and that is the whole door.** Guarding only
        `definition` and `version` would let a shelter clear `published_at`, edit the row
        freely as a draft, and republish — immutability bypassed without ever touching a
        guarded column. Same shape as T-01-020's cascade: the hole is never in the thing being
        watched. Mutant M4 allows the thaw and dies.
      - **TWO BUGS FOUND IN THE MIGRATION BEFORE IT EVER RAN**, both while re-reading it: `%%`
        is a literal percent in a plpgsql `RAISE`, not a placeholder; and **`RETURN NEW` in a
        `BEFORE DELETE` trigger returns NULL, which CANCELS THE STATEMENT SILENTLY** — no
        error, zero rows. Deleting a draft would have looked like it worked. The draft
        anti-vacuity now asserts `RowsAffected()`, not just the absence of an error, and mutant
        M5 confirms it catches the silent cancel.
      - **A GAP IN MY OWN TEST SET, FOUND WHILE DESIGNING THE MUTANTS:** nothing proved that
        deleting a template does not take its published versions with it. `app_tenant` holds
        DELETE on `form_templates`, so with `ON DELETE CASCADE` the shelter erases the form's
        history by erasing the form. `ON DELETE RESTRICT` closes it — retiring a form is what
        `is_active` is for — and `TestPublishedFormVersion_CannotBeErasedByDeletingItsTemplate`
        now asserts it, with the anti-vacuity that a template with NO versions is still
        deletable.
      - **A HAND-WRITTEN LIST IN THE POLICY INVENTORY WAS SILENTLY EXCLUDING NEW TABLES.** The
        inventory query filtered on a hardcoded `c.relname IN (...)`, so the two new tables'
        policies were never read at all — the test reported a clean schema it had not looked
        at, and the symptom was a confusing length mismatch rather than a missing policy. It is
        now bound to `rlstest.Schema.ModelTables()`, so a table cannot be outside the inventory
        without also being outside the declaration.
      - **MUTATION: 7/7, first round.** M1 the composite parent key loses `shelter_id` · M2 the
        version key goes global · M3 the template key goes global · M4 the unpublish bypass ·
        M5 the silent DELETE cancel · M6 the `BEFORE TRUNCATE` trigger is removed · M7 the
        parent key cascades.
      - verify: `go vet` clean · `golangci-lint` **0 issues** · `govulncheck` 0 called
        vulnerabilities · `make test-api-container` green across all five packages · ledger
        **13 of 19 verified**
      - est: 154
      - pilot: blacklisted (§7.2)
      - engram: `mascotapp/security/constraint-check-order` obs-380c510874e6712e · `mascotapp/convention/plpgsql-triggers` obs-19d7bb85bca38508 · `mascotapp/convention/inventory-from-declaration` obs-a2740164931022d6 (aprobados por el usuario 2026-08-31)

- [x] **T-01-024** · Form versioning and immutability assertions  — **DONE 2026-09-01**
      - spec: dynamic-forms-data / *A published form version is immutable*, *Field ids are
        stable for life*
      - tests: publish v1, submit an answer, publish v2 with a field removed → the v1 answer
        still renders against v1 · a published version cannot be updated or deleted · a draft
        version can
      - dod: the historical-readability scenario is asserted end to end, not implied
      - **INHERITED FROM T-01-023 — what is already proven and what is left.** Done here
        already: a published version refuses UPDATE of `definition`, of `version` and of
        `published_at`, refuses DELETE, refuses TRUNCATE, survives a superuser, and cannot be
        erased by deleting its template; a DRAFT is still editable and deletable, asserted on
        `RowsAffected()` rather than on the absence of an error. **What is left is the part
        this task is named for and nothing above touches: the END-TO-END historical-readability
        scenario** — publish v1, record a submission against it, publish v2 WITHOUT that field,
        and show the v1 answer still resolves against v1. That needs `form_submissions`
        (T-01-025), so either this task follows it or it asserts the version side only and says
        so.
      - Mint your own shelters with `freshTenant`; do not reuse `env.ShelterA`/`ShelterB`.
      - **Closed after T-01-025, which is what the missing half needed.**
        `TestARecordedAnswer_StillResolvesAgainstItsOwnVersionAfterTheFieldIsDropped` walks the
        scenario end to end: publish v1 with `has_other_pets`, record an answer against it,
        publish v2 without the field, and then assert the property from the RENDERER's path —
        submission → its own version → that version's fields — rather than from the versions
        table. `unresolvedAnswerKeys` walks §4.4's shape (sections → rows → fields) and matches
        on `field.id`, never on the document as text, because a text match would find the id in
        a label, a logic reference or a value and report a field that is not there.
      - **Its anti-vacuity is load-bearing and mutation says so.** The same resolver run against
        v2 MUST report exactly `[has_other_pets]`; without it, a query that matches nothing
        returns zero unresolved keys and reads as proof. Mutants M3 (v2 keeps the field), M4
        (the answer never carried it) and M5 (resolve against the latest version instead of the
        recorded one) all die on that one assertion.
      - **A mutant survived and found a door the migration named but nobody asserted.**
        `ON DELETE CASCADE` on `form_submissions`' version reference left the whole suite green:
        every case that deleted a version deleted a PUBLISHED one, so 00007's immutability
        trigger answered first and the reference was never exercised. The reference could have
        been cascading since the day it was written. 00008's own comment says RESTRICT is there
        for *"a DRAFT version that somehow collected submissions"* — and nothing requires
        `published_at IS NOT NULL` to record one. `TestADraftVersionCarryingAnswers_CannotBeDiscarded`
        now asserts it, with the anti-vacuity that an EMPTY draft still deletes (on
        `RowsAffected()`), which is what keeps the refusal about the reference instead of about
        drafts. The comment on the published case was corrected too: it pins the trigger, not
        the key, and now says so.
        Mutation second round: **5/5**.
      - est: 110
      - pilot: blacklisted (§7.2)
      - engram: — (candidates in `.engram/queue/`, pending approval)

- [x] **T-01-025** · Migration `00008_form_submissions.sql`  — **DONE 2026-09-01**
      - spec: dynamic-forms-data / *Submissions are tenant-scoped and bound to a version*
      - build: `form_submissions` with `shelter_id`, `answers JSONB`, nullable
        `answers_encrypted BYTEA`, `ip_hash` · GIN index on `answers` · composite FK to the
        template version
      - tests: A/B · the GIN index is used for a containment query
      - dod: no PII in plaintext columns beyond what §4.4 specifies; encryption is Phase 07
      - **Delivered.** `00008_form_submissions.sql`: `form_submissions`, the GIN index on
        `answers`, the `(shelter_id, submitted_at DESC)` listing index, `tenant_isolation`,
        `REVOKE TRUNCATE` + the four DML grants, and the `UNIQUE (id, shelter_id)` this
        migration had to add to `form_template_versions` — declared in 00008 rather than in
        00007 because 00007 had already run. Tests in `form_submissions_test.go`: the plan for
        `answers @> ...` **names** `form_submissions_answers_idx` (planned as the OWNER, with
        an anti-vacuity case proving the same index is NOT dragged into a `LIKE` it cannot
        serve), the §4.4 column contract failing in both directions, and the bidirectional
        `application_id` deferral guard for T-01-027. A/B case added; ledger now **14 of 19**.
      - **Two mutants survived the first round and were CLOSED, not accepted.**
        **M2** — reducing the version reference to `REFERENCES form_template_versions (id)`
        — survived the whole suite: referential checks bypass row security, so the
        single-column key resolves another shelter's version on this tenant's behalf and
        nothing that READS can notice. Two things were missing and both landed:
        `TestFormSubmissions_CannotReferenceAnotherTenantsVersion` (behavioural, asserting the
        refusal is INDISTINGUISHABLE from a version that does not exist) and
        `TestTenantChildren_ReferenceTheirParentCompositely` (structural, driven by
        `Schema.TenantChildren`, so the four children still to land are checked the moment
        they appear). `form_submissions` had also never been added to `TenantChildren` —
        fixed in `catalog.go`, in the count (6→7) and in the spec's table.
        **M5** — deleting `REVOKE TRUNCATE` — is a **proven equivalent mutant**: no
        statement in this schema ever grants TRUNCATE, 00001 declares there is no
        `ON ALL TABLES` and no `ALTER DEFAULT PRIVILEGES`, and PostgreSQL grants none to
        PUBLIC by default, so the REVOKE reaches the same privilege state as its own absence.
        The REVOKE stays (it fires the day the schema acquires default privileges); the
        MUTANT was replaced by one that actually widens the state, and the class it pointed at
        was closed by `TestNoApplicationRole_HoldsTruncateOnAnyTable`, which enumerates the
        catalog instead of trusting three hand-written inventory rows.
        Second round: **7/7**.
      - est: 72
      - pilot: blacklisted (§7.2)
      - engram: — (candidates in `.engram/queue/`, pending approval)

- [x] **T-01-026** · Forms orphan and index assertions  — **VERIFICATION PASS, DONE 2026-09-01**
      - spec: dynamic-forms-data / *A submission cannot reference another tenant's version*
      - tests: the `23503` orphan case for `form_template_versions` and `form_submissions` ·
        `EXPLAIN` shows the GIN index in use
      - **ALREADY DELIVERED BY EARLIER TASKS — read this before starting.** All three items
        of the list above exist and pass today, each written under a different task because
        the mutation round of that task demanded it, not because scope was borrowed:
        - the orphan case for `form_template_versions` — `form_templates_test.go` ·
          `TestFormTemplateVersions_VersionKeyIsNotACrossTenantOracle` and the composite-key
          refusal it pairs with (T-01-023)
        - the orphan case for `form_submissions` — `form_submissions_test.go` ·
          `TestFormSubmissions_CannotReferenceAnotherTenantsVersion`, plus the enumerated
          `TestTenantChildren_ReferenceTheirParentCompositely` (T-01-025, closing mutant M2)
        - `EXPLAIN` shows the GIN index in use — `form_submissions_test.go` ·
          `TestFormSubmissions_AnswersAreSearchableByContainment`, which asserts the plan
          **names** the index rather than matching the string `"Index Scan"` (T-01-025)
        So this task is either a **verification pass** — re-read those three against the spec
        and close it — or it is where something genuinely missing gets named. What it is NOT
        is a rewrite of assertions that already exist.
      - **VERIFIED 2026-09-01.** Every scenario of `dynamic-forms-data` mapped to a test by
        name and run, not matched by memory:

        | Spec scenario | Test | |
        |---|---|---|
        | Editing a published definition is refused (+ *stored definition unchanged*) | `TestPublishedFormVersion_CannotBeEditedOrDeleted/editing_the_definition` + the trailing `schemaVersion` read-back | ✅ |
        | Deleting a published version is refused (+ *the row remains*) | `.../deleting_the_row`; "remains" is asserted **transitively** — the read-back below it errors if the row is gone | ✅ |
        | A draft version is still editable | `.../a_draft_version_is_still_editable`, on `RowsAffected()` | ✅ |
        | Publishing creates a new version rather than mutating | `TestPublishingAVersion_AppendsAndLeavesTheOldOneByteIdentical` (3 subtests) | ✅ |
        | Answers survive a field removal | `TestARecordedAnswer_StillResolvesAgainstItsOwnVersionAfterTheFieldIsDropped` | ✅ |
        | Duplicate field identifiers are rejected | — | ➡️ **moved to Phase 07** |
        | An unknown field type is rejected | — | ➡️ **moved to Phase 07** |
        | An out-of-range span is rejected | — | ➡️ **moved to Phase 07** |
        | The three tables satisfy the isolation rule | `TestTenantIsolation/form_templates`, `/form_template_versions`, `/form_submissions` | ✅ |
        | A version cannot be attached to another tenant's template | `TestFormTemplateVersions_VersionKeyIsNotACrossTenantOracle` | ✅ |
        | Template keys are unique per shelter, not globally | `TestFormTemplateKeys_AreUniquePerShelterNotGlobally` | ✅ |
        | Answers are searchable | `TestFormSubmissions_AnswersAreSearchableByContainment` — stronger than asked: the plan must **name** the index, not merely have it in the catalog | ✅ |

      - **The task's own three items were already delivered and all three pass**, each written
        under the task whose mutation round demanded it: the orphan case for
        `form_template_versions` (T-01-023), the one for `form_submissions` plus the enumerated
        `TestTenantChildren_ReferenceTheirParentCompositely` (T-01-025), and the `EXPLAIN`
        assertion (T-01-025). Nothing was rewritten.
      - **Two spec amendments, both drift between a deliberate decision and the document.**
        (1) *Publishing creates a new version* still wrote `UNIQUE (template_id, version)`;
        T-01-023 deviated to `UNIQUE (shelter_id, template_id, version)` **with evidence** — the
        unique index is checked before the foreign key, verified live on PG 17, so the literal
        key is a cross-tenant existence oracle even with the composite FK present, and the
        mutant restoring it dies. The reasoning lived in code comments and in this board; the
        spec now carries it. (2) *Templates, versions and submissions are tenant-scoped* spelled
        out the composite reference for `form_template_versions → form_templates` and was
        **silent about `form_submissions → form_template_versions`** — precisely the hole
        T-01-025's mutant M2 exploited. The requirement now states it, and the capability gained
        the submission's own orphan scenario, including the indistinguishability clause.
      - **One finding was a scope decision, and the user resolved it 2026-09-01: moved to
        Phase 07.** *Duplicate field identifiers*, *unknown field type* and *out-of-range span*
        are written as `WHEN it is validated`: they are **domain validation, not schema**, and
        nothing in this repository validates a definition — no `packages/form-schema/`, no Go
        validator, no `CHECK` over `definition`'s shape, and no task on this board. The plan puts
        the shared closed union and the form engine in Phase 07.
        The move was **surgical, not wholesale**. From *Field identifiers are stable* only the
        validation half left — the `SHALL be unique within its template` clause and its scenario
        — while *Answers survive a field removal* **stayed**, because that half the database does
        enforce and T-01-024 asserts it end to end. *Field types are a closed union* moved
        entire: its requirement text is validation from top to bottom. Both are recorded
        **verbatim** in `docs/vault/30-fases/FASE-07.md` under "Alcance heredado de Fase 01", so
        `/sdd-new fase-07` picks them up rather than someone remembering.
        A jsonb `CHECK` was considered and rejected as the wrong layer **as the only layer**: a
        `CHECK`'s error cannot name the duplicated identifier or the unsupported type, which is
        what the scenarios ask for.
        **The constraint that travels with them:** validation must land BEFORE or WITH the first
        template write path. Published versions are frozen by a trigger, so an invalid definition
        that gets published **cannot be repaired** — only superseded. The window is closed today
        by phase ordering, and Phase 07 is the phase that opens it.
        With this delta scoped to what it delivers, `/sdd-verify` on this change has no
        unenforced `SHALL` left.
      - est: 45
      - pilot: blacklisted (§7.2)
      - engram: —

- [x] **T-01-027** · Migration `00009_adoption_applications.sql`  — **DONE 2026-09-01**
      - spec: adoption-flow-data / *Applications are tenant-scoped*
      - build: `adoption_applications` with the §4.5 status enum, `assigned_to_user_id`,
        `priority_score`, `decision_note` · composite FK to `pets` · the §4.6 index ·
        **the applicant branch of the `users` policy** (D6), which could not exist at `00002`
        because this table did not
      - tests: A/B · an applicant sees their own application
      - dod: status **transitions** are domain-layer and explicitly out of scope here
      - **Written test-first.** All five cases ran RED against the missing table (`42P01`)
        before the migration existed. Ledger now **15 of 19**, A/B suite at **11** cases.
      - **Delivered.** `adoption_applications` with §4.5's closed status set, the §4.6 listing
        index, `tenant_isolation`, `REVOKE TRUNCATE` + the four DML grants, the
        `UNIQUE (id, shelter_id)` its own children will reference at T-01-029/031, and the
        applicant branch of the `users` policy (D6) — a SECOND permissive policy, not a widened
        first one, because permissive policies OR together and one policy with an OR inside it
        is a single edit away from widening both paths at once.
      - **`assigned_to_user_id` is a composite key to `memberships (user_id, shelter_id)`, not
        a reference to `users`.** `memberships` already carries that UNIQUE for its own sake,
        which is exactly the referenced key D5 wants — so *"assignment stays inside the
        shelter"* becomes something the database cannot be talked out of rather than a rule a
        handler remembers. No policy could have caught it: the tenant writes the value into its
        OWN row and `WITH CHECK` only looks at `shelter_id`. MATCH SIMPLE skips the check when
        the column is NULL, so an unassigned application stays unconstrained. Its own assertion
        is here; the fuller grid stays T-01-028's.
      - **The bidirectional guard from T-01-025 went red by itself and named exactly what to
        do.** `TestFormSubmissions_GetsItsApplicationKeyWhenApplicationsLand` fired the moment
        this migration created the table, so `form_submissions.application_id` got its composite
        key here. **`ON DELETE CASCADE`, the opposite of every other reference in this schema,
        and deliberately:** elsewhere the child is the history and the parent is the live row,
        so a cascade lets a tenant erase the trail; here the child is the PERSONAL DATA and the
        parent is the case, and §5.4's retention purge and subject-deletion path are precisely
        the answers going away. A RESTRICT would leave the adopter's document behind after the
        case was purged. It is safe only because the audit trail does not live there —
        `application_events` (T-01-029) and `audit_log` (T-01-032) are append-only and separate,
        and **both must reference this table with RESTRICT for the reason this one does not.**
      - **Three mutants survived the first round; all three were CLOSED.**
        **M2** — `ON DELETE CASCADE` on the pet reference — broke no test. `app_tenant` holds
        DELETE on `pets`, so a shelter could erase every application it ever received with one
        statement. Third time this phase the hole was in what happens to a table when its PARENT
        goes (`pet_status_history` T-01-020, the draft version T-01-024).
        `TestApplications_CannotBeErasedByDeletingTheirPet` closes it, with the anti-vacuity
        that a pet with no applications still deletes.
        **M10** — `form_submissions.application_id` reduced to a single column — survived the
        suite **including the enumerated child guard**, because that guard asked whether a child
        has A composite reference and the table still had its key to the version. Two fixes:
        `TestFormSubmissions_CannotBindToAnotherTenantsApplication` behaviourally, and the guard
        itself hardened to check **every** reference to a tenant table, exempting only the
        table's own `shelter_id → shelters`.
        **M7** — the applicant policy's `a.shelter_id` predicate removed — survived, and is a
        **proven equivalent mutant given the current schema**: a policy's `EXISTS` subquery is
        itself filtered by the referenced table's RLS (T-01-020 mutant M13), and
        `adoption_applications` carries `tenant_isolation`, so the subquery already returns
        nothing for another shelter. The predicate STAYS — without it this policy's isolation is
        borrowed from a policy on a different table, and widening that one would widen this one
        silently — and is pinned by
        `TestApplicantPolicy_ScopesItselfRatherThanBorrowingTheChildsPolicy`, which asserts both
        branches name `app.shelter_id` in their own `USING`. A twelfth mutant covers the
        behavioural half (drop the correlation to `users.id`) and dies on the grid.
        Second round: **12/12**.
      - **KNOWN EXPOSURE, recorded not hidden.** D6's rule — *write permission on the bridge is
        read permission on the table* — applies to the applicant policy, and unlike `memberships`
        there is no business state to filter on: the shelter writes the status too, so a forged
        row can simply say `submitted`. A tenant that already knows a user's uuid can mint read
        access to their PII. The real fix is column-level grants, which is Judgment Day's
        finding B1, deferred by the user to Phase 02/03.
        `TestApplicantPolicy_IsAsWideAsWritingAnApplication` is the characterization test that
        goes red the day B1 is paid.
      - est: 117
      - pilot: blacklisted (§7.2)
      - engram: — (candidates in `.engram/queue/`, pending approval)

- [x] **T-01-028** · Adoption-flow domain assertions  — **DONE 2026-09-01**
      - spec: adoption-flow-data / *Applications are tenant-scoped*
      - tests: the applicant `users` policy does not widen tenant visibility · assignment stays
        inside the shelter
      - **INHERITED FROM T-01-027 — already proven, not repeated here.** The applicant
        visibility grid (visible to the shelter applied to, invisible to another, a member still
        visible, and a user who neither applied nor belongs still invisible), the structural pin
        that BOTH branches name `app.shelter_id` in their own `USING`, the base case that
        assigning to another shelter's member is refused, and the characterization of the known
        exposure (`TestApplicantPolicy_IsAsWideAsWritingAnApplication`).
      - **Delivered here** — `adoption_flow_test.go`:
        - `TestApplicantVisibility_EndsWhenTheApplicationDoes` — D6's rule read in the other
          direction. The membership branch got `m.status = 'active'` in T-01-016 so that
          REVOKING takes the visibility away; the applicant branch's equivalent of revocation is
          the application going away, and §5.4 gives a subject the right to have their data
          removed. Visibility outliving the application would mean "delete my data" left the
          shelter still able to read the person.
        - `TestAssignmentOnInsert_CannotLeaveTheShelter` — the same constraint on the INSERT
          path, because a form that creates an already-assigned application never touches the
          update handler. A rule holding on one statement and not the other is found in
          production, by whichever handler was written second.
        - `TestAnAssignedMembership_CannotBeDeleted` — `ON DELETE RESTRICT` on the assignee key.
          Under `SET NULL` the case quietly drops out of a queue; under `CASCADE` the
          application itself goes. Both mutants die here.
      - **A REAL INCONSISTENCY FOUND AND RECORDED, not smoothed over.** A shelter can assign a
        case to a member it CANNOT READ. `member_visible_users` filters on `status = 'active'`;
        the composite key can only check that the membership pair EXISTS — and that is
        PostgreSQL, verified on 17 rather than assumed: **a foreign key cannot reference a
        partial unique index** (`there is no unique constraint matching given keys`), so
        `UNIQUE (user_id, shelter_id) WHERE status = 'active'` is not available as the
        referenced key. So `assignable = the row exists` while
        `readable = the row exists AND is active`. It is not a tenant leak — everyone involved
        belongs to this shelter — so the database's answer is incomplete rather than wrong, and
        closing it needs a trigger or the domain layer, where the assignment rules live with
        RBAC in Phase 02. `TestAssignment_DoesNotYetRequireAnActiveMembership` records it and
        goes red the day it is narrowed. Raised in FASE-01's open questions.
      - Mutation: **5/5, first round** — the first task of this phase to need no second one. M5
        is the cross-check that matters: removing `m.status = 'active'` from 00002 fails the
        characterization test's own premise, so the inconsistency it records cannot quietly stop
        being one.
      - est: 90
      - pilot: eligible
      - engram: — (candidates in `.engram/queue/`, pending approval)

- [x] **T-01-029** · Migration `00010_application_children.sql` — append-only events and notes  — **DONE 2026-09-01**
      - spec: append-only-audit / *Application events are append-only*
      - build: `application_events` (append-only) and `application_notes`, both with a
        denormalised `shelter_id` and composite FKs (D5) · append-only enforced by **all three**
        layers: revoked UPDATE/DELETE grants, no permissive policy for those commands, and a
        trigger — because the table owner would otherwise bypass the first two
      - tests: A/B for both · UPDATE, DELETE and TRUNCATE all **fail**
      - dod: assert failure, not zero rows affected
      - **Written test-first.** All four cases ran RED against the missing tables (`42P01`).
        Ledger now **17 of 19**, A/B suite at **13** cases, only `documents` (T-01-031) and
        `audit_log` (T-01-032) still pending.
      - **The two tables are NOT the same kind of thing, and that is the design.**
        `application_events` is the TRAIL — append-only under four layers, because a record of
        what happened that can be rewritten is not a record of what happened.
        `application_notes` is WORKING MEMORY — a person writing *"called, no answer"*, fully
        editable, because people make typos and change their minds. Treating notes as evidence
        would freeze the shelter's own scratchpad; treating events as notes would leave the
        audit trail rewritable by whoever wants it to say something else.
        `TestApplicationNotes_AreEditableUnlikeTheEventTrail` is what keeps "append-only"
        meaning something rather than describing every table this migration created.
      - **The obligation T-01-027 wrote down, paid.** Both children reference
        `adoption_applications` with **`ON DELETE RESTRICT`**, and that is what makes 00009's
        `CASCADE` on `form_submissions.application_id` safe. There the child is the PERSONAL
        DATA and §5.4's purge is precisely the answers going away; that is only defensible
        while the EVIDENCE lives somewhere that does not cascade. With both cascading, purging
        a rejected solicitud would erase the record that it ever existed — who reviewed it,
        when, what was decided. Mutants M5 and M6 die on it.
      - **`type` is deliberately NOT a closed union, and `visibility` deliberately is.** The
        rule that decides it is the same one in both directions: a set is closed when the
        domain BRANCHES on it. A note's `visibility` decides who may read the note, so a third
        value reaches a switch with no arm for it and the fallback decides whether an adopter
        sees what the shelter wrote about them. An event `type` is the opposite shape — the
        domain EMITS it, an unrecognised one is ignored, and the set grows with every feature
        that records something. A `CHECK` there would mean a migration per event type, which
        is how a timeline stops being written.
      - **Mutation: 11/11, first round — and the shape of the result is the point.** Four
        mutants each removed exactly ONE layer of the append-only stack, and each died on a
        DIFFERENT test: the row trigger (M1) on the owner cases, the truncate trigger (M2) on
        the truncate case, the grants (M3) on the inventory and the A/B case, the split
        policies (M4) on the policy inventory. If any one of them had survived, that layer was
        never protecting anything and the other three were carrying it.
      - **A coupling that fixed itself.** `TestCheckCatalogAt_FailsOnABrokenIntermediateSchema`
        creates a probe table named from the pending ledger, and it broke the moment these two
        landed while still listed as pending — with `42P07`, "relation already exists". That is
        the ledger doing its job: it was rewritten in T-01-019 to read the ledger instead of a
        literal `pets` precisely so this failure means "update Pending", not "pick another
        name". Deleting the two lines fixed both it and the meta-test.
      - est: 174
      - pilot: blacklisted (§7.2)
      - engram: — (candidates in `.engram/queue/`, pending approval)

- [x] **T-01-030** · Append-only enforcement suite  — **DONE 2026-09-01, as the enumerated suite**
      - spec: append-only-audit / *Append-only tables refuse modification*
      - tests: for `application_events` and `audit_log`, UPDATE/DELETE/TRUNCATE fail with
        `42501` or the trigger's exception · the owner cannot bypass it either
      - dod: the owner case is asserted — it is the one the grants alone do not cover
      - **HALF ALREADY DELIVERED, read this before starting.** `application_events` has its
        full stack asserted by T-01-029:
        `TestApplicationEvents_AreAppendOnlyEvenForARoleThatBypassesRowSecurity` runs as the
        OWNER (a superuser here) for UPDATE, DELETE and TRUNCATE, distinguishes the trigger's
        `P0001` from the grant's `42501` so one layer cannot cover for another, asserts the row
        is unchanged afterwards, and carries the anti-vacuity that the owner CAN still append.
        The A/B case runs with `AppendOnly: true`, and the grant and policy inventories carry
        its rows.
        **`audit_log` does not exist until T-01-032**, so this task cannot cover its half
        either — which is the real question to answer when it is opened: either it runs after
        T-01-032, or it becomes the ENUMERATED suite over a declared append-only set, so the
        two tables (and any third) are covered by one rule rather than by two hand-written
        cases. The second is what this phase has learned to prefer.
      - **RESOLVED: both, and in that order.** The user reordered the board to
        T-01-031 → T-01-032 → T-01-030 for the reason above — running this task before
        `audit_log` existed meant writing nothing for one table and a silent no-op for the
        other, which is exactly what the pending ledger exists to prevent.
      - **Delivered as `append_only_test.go`, driven by `Schema.AppendOnly`** — a new declared
        set with the same two consistency checks `TenantChildren` has (a name outside the model
        set, and a duplicate), so the declaration cannot quietly stop meaning anything.
        - `TestAppendOnlyTables_RefuseEveryRewriteEvenForTheOwner` runs UPDATE, DELETE and
          TRUNCATE **as the OWNER** against every declared table. That actor is the point: of
          the four layers, only the triggers stop a role carrying `BYPASSRLS`, and on Neon
          `neon_superuser` carries it. It asserts `P0001` specifically rather than "an error",
          which is what stops one layer covering for another; it carries the anti-vacuity that
          the owner CAN still append, on a row COUNT rather than on the absence of an error; and
          it re-reads the count afterwards, because a refusal that still wrote is the worst
          outcome of all.
        - `TestAppendOnlyTables_CarryNoMutatingPolicy` is the layer that **cannot be
          exercised**, because its enforcement is an ABSENCE — the triggers refuse first, so no
          behavioural probe can tell an absent UPDATE policy from a present one. The catalog is
          the only place that answer lives. It also asserts the SELECT and INSERT policies ARE
          there, or the table would be unusable rather than append-only.
      - **Adding a third append-only table now means adding its name to the declaration and its
        fixture. Nothing else.** Mutants M3–M6 of T-01-032 each remove one layer of `audit_log`'s
        stack, and each dies in this suite — a table that was never written about by hand.
      - est: 85
      - pilot: blacklisted (§7.2)
      - engram: —

- [x] **T-01-031** · Migration `00011_documents.sql`  — **DONE 2026-09-01**
      - spec: adoption-flow-data / *Documents reference a media object and may stand alone*
      - build: `shelter_id`, required `media_id` with composite FK `(media_id, shelter_id)`,
        **nullable** `application_id`, `type` constrained to `adoption_contract`, `receipt`,
        `health_certificate`, `custom` · standard policy template
      - tests: a row with `application_id` null succeeds · under scope B, referencing A's media
        fails with `23503`
      - dod: composite FK `(media_id, shelter_id)` in place · `media`'s `UNIQUE (id, shelter_id)`
        is created in `00003` (T-01-017), **not** added here — do not `ALTER` a table eight
        migrations upstream
      - **Written test-first**; the DoD's precondition was verified before writing, not assumed:
        `media` has carried `UNIQUE (id, shelter_id)` since 00003 line 54, so nothing was
        ALTERed upstream. Ledger now **18 of 19** — only `audit_log` (T-01-032) is left.
      - **`application_id` is nullable AND half of a composite key, and that combination is the
        design rather than a compromise.** PostgreSQL's default MATCH SIMPLE skips the ENTIRE
        check when any column of the key is NULL, so a standalone document is possible without
        weakening the key for the rows that do carry one. `MATCH FULL` — same columns, same
        tables — refuses the standalone case outright, and mutant M4 proves the difference is
        load-bearing rather than a detail.
      - **Both references RESTRICT**, completing the rule 00010 established: 00009 let
        `form_submissions` CASCADE because there the child is the PERSONAL DATA, and that is
        only defensible while the EVIDENCE does not. An adoption contract is the record of who
        took which animal home — a cascade would erase it while leaving the pet marked
        `adopted`, and the shelter would have no record of who has it.
      - **A mutant survived and found a column nothing guarded.** Dropping `NOT NULL` from
        `media_id` broke no test, and it costs more than an empty row: `media_id` is half of the
        composite key to `media`, so by the same MATCH SIMPLE rule above, a NULL there turns
        **D5's tenant check off for that row**. `TestDocuments_RequireTheirFile` closes it and
        asserts `23502` specifically — the column is both NOT NULL and a foreign key, and only
        the SQLSTATE says which layer answered.
        Second round: **9/9**.
      - **A TENSION THIS MIGRATION MAKES CONCRETE — raised in FASE-01's open questions.** With
        `application_events`, `application_notes` and now `documents` all RESTRICT, an
        application that has ANY history **cannot be hard deleted**. So §5.4's retention purge
        cannot be "DELETE the application" — it has to be data MINIMISATION over a row that
        stays. T-01-027's comment justifying `form_submissions`' CASCADE by that purge is
        therefore incomplete, and `applicant_user_id` is `NOT NULL`, so the application row
        cannot be anonymised in place either. What survives a purge is a product decision.
      - est: 77
      - pilot: blacklisted (§7.2)
      - engram: — (candidates in `.engram/queue/`, pending approval)

- [x] **T-01-032** · Migration `00012_audit_log.sql`  — **DONE 2026-09-01**
      - spec: append-only-audit / *The audit log is append-only and tenant-scoped*
      - build: `audit_log` with `BIGSERIAL` id, `before`/`after` JSONB, `ip_hash`, `user_agent` ·
        the §4.6 index on `(shelter_id, occurred_at DESC)` · append-only in all three layers ·
        **the sequence grant**, which a table grant alone does not cover
      - tests: A/B · append-only · `app_tenant` can insert, which requires USAGE on the sequence
      - dod: forgetting the sequence grant is caught by a test, not by production
      - **Written test-first. The ledger is now EMPTY: 19 of 19 declared tables verified**, so
        every protection assertion in this package runs against a relation that exists. A/B
        suite at **14** cases.
      - **The DoD, delivered and then hardened by mutation.** `bigserial` is not a type — it is
        `bigint` plus a SEQUENCE plus a default calling `nextval`. A table grant says nothing
        about that sequence, so without `GRANT USAGE ON SEQUENCE` the tenant can insert only
        when it supplies an id, which every test that writes explicit ids does and no real
        caller does: invisible in development, arrives on the first production write. Mutant M1
        removes the grant and dies.
      - **A mutant survived and found the OTHER half of that grant.** `GRANT ALL` instead of
        `GRANT USAGE` broke no test — and `ALL` on a sequence includes `UPDATE`, which is
        `setval`. A tenant could set the counter high, append, set it back low and append again,
        so a LATER audit row carries a SMALLER id than an earlier one — and the log is read in
        id order precisely because that order is meant to be the order things happened. It could
        also rewind into taken ids and turn every later append into a primary key collision,
        which is an audit trail that stops recording.
        `TestAuditLog_TheTenantCannotMoveTheSequence` closes it, with the anti-vacuity that
        `nextval` still works — otherwise a missing USAGE grant would pass the refusal while
        making the table unwritable, M1 passing as if it were M2.
        Second round: **9/9**.
      - **`entity_type` / `entity_id` are polymorphic and deliberately unkeyed**, with a test
        that says so. A foreign key names ONE table, so it would either refuse every audit row
        about anything else or become a single-column cross-tenant reference (D5). What contains
        the hole is that this is a LOG, not a reference: nothing resolves `entity_id`, and the
        row is tenant-scoped by its own `shelter_id`. Mutant M9 bolts a `REFERENCES pets (id)`
        on and dies.
      - est: 162
      - pilot: blacklisted (§7.2)
      - engram: — (candidates in `.engram/queue/`, pending approval)

- [x] **T-01-033** · sqlc query inputs and committed generated output  — **DONE 2026-09-01**
      - spec: schema-migrations / *Generated code is committed and verified*
      - build: `internal/db/query/*.sql`, one file per aggregate · `sqlc generate` · commit
        `internal/db/sqlcgen/`
      - tests: `make generate && git diff --exit-code`
      - dod: generated output is committed (D2) and excluded from the authored-line count, but
        stays in snapshot identity
      - est: 200 authored SQL (**the softest number in this file** — no artifact states which
        queries Phase 01 owes, since handlers arrive in Phase 02; scoped to a minimal set
        exercising every aggregate)
      - **The soft estimate, made checkable.** 9 query files, **254 authored lines**, 12
        generated. The rule that replaces taste: `TestQueries_ExerciseEveryDeclaredTable`
        requires **every declared model table to be named by at least one query**, with ONE
        exemption **derived** from `Schema.NoPolicy` rather than hand-written —
        `refresh_tokens` is default-deny, so a query against it could only ever fail. The
        exemption expires on its own: the day Phase 02 gives that table an access path, it
        leaves `NoPolicy` and the test starts demanding a query for it. It fails in the other
        direction too, so a query against a default-deny table is caught as well.
      - **The deferral guard from T-01-012 fired exactly as designed.**
        `TestSQLC_IsWiredIntoGenerateExactlyWhenQueriesExist` had been asserting that sqlc is
        wired into `make generate`, CI and the devcontainer **exactly when query files exist** —
        and it named all three the moment the first `.sql` landed. The devcontainer already
        installed sqlc pinned at v1.31.1; the Makefile and CI got their steps, CI pinned to the
        same version.
      - **A CONVENTION ESTABLISHED HERE, and it is a safety rule dressed as a style one: a
        query does not filter by `shelter_id`. The policy does.** The whole argument for RLS is
        that a forgotten `WHERE shelter_id = ?` returns zero rows instead of another shelter's
        data. A query that filters by it ITSELF inverts that — it returns the same rows whether
        or not the policy exists, so dropping a policy becomes invisible in every such query and
        surfaces only through some other one that forgot. Belt-and-braces is the wrong instinct:
        the braces must not be able to hide the belt's absence. INSERTs are exempt and must be —
        supplying a NOT NULL column is not filtering by it.
      - **I OVERCLAIMED IN A COMMENT AND MUTATION-STYLE PROBING CORRECTED IT.** The first draft
        said sqlc catches schema drift, full stop. Probed against v1.31.1 rather than assumed:
        renaming a column a query NAMES fails generation on the exact file:line
        (`column "display_name" does not exist`); renaming one only reached through `SELECT *`
        does NOT — generation succeeds and `models.go` returns with the new shape, moving the
        failure to whoever reads the removed field. Neither is silent, but the claim was half
        true and the comment now says which half.
      - **Three mutants survived the first round and all three exposed the SAME defect, one test
        apart.** M7 showed the wiring guard's CI check — `strings.Contains(ci, "sqlc")` — was
        satisfied by a **comment**, so a workflow that installs the tool and never runs it, or
        runs it and never compares, passed. Strengthened to require both `sqlc generate` and the
        `git diff --exit-code -- internal/db/sqlcgen`. Then M1 and M2 removed `pet_media`'s and
        `breeds`' ONLY queries and the coverage test stayed green — because each table was still
        named **in a comment**. Fixed by stripping `--` comments before counting, plus
        `TestQueries_UseOnlyTheCommentSyntaxTheStripperHandles`, which states the stripper's
        limit (no block comments, no `--` inside a literal) so a deliberate shortcut cannot
        become an accidental bug.
        **A substring found anywhere in a file is not an assertion about what the file does** —
        and this task made that mistake twice before catching it.
        Second round: **9/9**.
      - est: 200 authored SQL → **254 actual**
      - pilot: eligible
      - engram: — (candidates in `.engram/queue/`, pending approval)

- [x] **T-01-034** · ADR-0007 … ADR-0011  — **DONE 2026-09-01**
      - spec: — (documentation)
      - build: `docs/vault/20-arquitectura/` — ADR-0007 child denormalisation + composite tenant
        FKs (D5/D6) · ADR-0008 `WithTenant` as the only door (transaction-scoped GUC, no
        pool-level binding) · ADR-0009 roles, grants and passwords by migration; default-deny
        (D8/D9) · ADR-0010 append-only tables and why all three layers are needed · ADR-0011
        extension policy: UUIDv7 in Go because `pg_uuidv7` is unavailable on Neon, `citext` used
        because it is verified present in both environments (D3/D7)
      - dod: numbering starts at **0007** — 0001..0006 are taken
      - **The five landed**, each with the rejected alternatives and the reopening conditions
        the template asks for, and each carrying what the mutation rounds of this phase actually
        found rather than what the design predicted:
        - **ADR-0007** — the composite-key rule, with the fact that decides it stated as a fact:
          referential checks ALWAYS bypass row security. Records that a single-column key
          survived the whole suite twice (T-01-025, T-01-027), that the property is
          INDISTINGUISHABLE refusal rather than refusal, and that the `ON DELETE` direction is
          decided by what the child IS — evidence goes RESTRICT, personal data goes CASCADE, and
          a child that is both is mis-modelled.
        - **ADR-0008** — `WithTenant` as the only door, with both facts that fixed its form:
          `SET LOCAL` takes no bind parameters, and a GUC reverted at COMMIT returns the EMPTY
          STRING, which is why every policy in this schema reads
          `nullif(current_setting(...), '')`. Carries the query convention it implies.
        - **ADR-0009** — roles, passwords and default-deny, including the sequence grant that is
          the only grant in the schema not about a table, and B1 named as the most likely
          reopening.
        - **ADR-0010** — append-only. **The board said "three layers"; it is FOUR** — the
          `TRUNCATE` trigger is not a variant of the row trigger, because a `FOR EACH ROW`
          trigger never fires on a statement that visits no rows. Records how the four are
          proven independent (one mutant per layer, each dying in a DIFFERENT test) and that
          layer 1 cannot be exercised at all, because its enforcement is an absence.
        - **ADR-0011** — the extension policy, and the correction that produced it: D3's
          *"no dependemos de extensiones"* generalised from ONE true case (`pg_uuidv7` really is
          absent on Neon) to all, and had already bent the schema away from §4 before anybody
          checked. Each extension is verified in BOTH environments, separately.
      - **Guards added, because a document with no assertion drifts.** `adr_test.go`:
        numbering unique and contiguous (the DoD, kept true for the phase that has not read
        this task), every `[[ADR-NNNN]]` link resolving to a file that exists — a dangling wiki
        link renders as plain text in Obsidian and as nothing in a diff, and the vault is the
        CONTINUITY layer — and the universal header and sections.
      - **The first draft of that last guard was wrong, in the exact mirror of T-01-033's
        lesson.** It required a literal `## Alternativas descartadas` heading and failed all
        four Phase 00 ADRs — which are NOT missing the reasoning: ADR-0004 puts it in its
        channel-by-channel table and "Por qué en VISA sí y aquí no", ADR-0005 in "El criterio de
        aptitud", ADR-0003 in "Riesgo aceptado". A string check there measures FORM and calls it
        substance. Narrowed to what is universal across all eleven, with the reason written down
        so the next person does not re-add it.
      - Mutation: **7/7, first round** — a number reused, a gap, a dangling link, a self-link, and
        three dropped sections.
      - est: 225 (documentation)
      - pilot: eligible
      - engram: — (candidates in `.engram/queue/`, pending approval)
      - parallel: may run alongside any task after its subject has landed

- [x] **T-01-035** · Phase board and continuity backfill  — **DONE 2026-09-01**
      - build: `docs/vault/30-fases/FASE-01.md` final state · `PROJECT_STATE.md` · a
        `docs/vault/40-bitacora/` entry · Engram queue candidates evaluated against
        `.engram/RUBRIC.md`
      - dod: every task `[x]` · **the SEVEN open questions** either answered or explicitly
        carried to Phase 02 · queue empty or approved
      - **Los tres items del DoD, cumplidos.**
        1. **Toda tarea en `[x]`** — 35 de 35, en los dos tableros.
        2. **Las siete preguntas abiertas, cerradas.** Dos las decidió el usuario; cinco se
           arrastraron con destino escrito, no con una nota. **Arrastrar sin destino es perder**
           (lección de T-01-026), así que cada una quedó **textual** en la fase que la recibe:
           `refresh_tokens`, B1 y la asignación a miembro no activo en `[[FASE-02]]`;
           `app_public` sobre `shelters` y `pets.breed_id` en `[[FASE-06]]`. Smart App Control
           salió de la lista: es una decisión de entorno del usuario, permanente, y **Fase 01 no
           tiene nada que decidir al respecto**.
        3. **Cola de Engram vacía.** 52 guardados, 2 descartados, 0 pendientes.
      - **LT-2 → FASE 06, y el motivo estaba sin decir.** El tablero ofrecía Fase 01 o Fase 10.
        **Fase 10 es POST-MVP** (el corte es al final de la 08), así que diferirla ahí significaba
        **lanzar el MVP con el vector de estafa abierto** — exactamente lo que LT-2 existe para
        evitar. Va a Fase 06, junto con la política de `app_public` sobre `shelters` que la
        condición **necesita** para no romper el catálogo.
      - **La purga de retención → borra las respuestas, conserva el caso.** La `CASCADE` de 00009
        ya lo hace, así que no cuesta migración. **El costo, dicho y aceptado:**
        `applicant_user_id` es `NOT NULL`, así que el vínculo al titular sobrevive — **no es un
        borrado completo del sujeto**, y las dos salidas alternativas quedan escritas en
        `[[FASE-08]]`. **Y corrige a T-01-027**: el comentario que justificaba la CASCADE POR esa
        purga está incompleto, porque con historia presente esa cascada no se dispara nunca.
      - **UN HALLAZGO PROPIO DEL CIERRE, y es la lección de la fase aplicada a la fase misma.**
        La verificación de §10 exige un caso A/B por cada tabla con `shelter_id`, y es
        **BLOQUEANTE**. Se comparó la lista de casos contra el set declarado **A OJO** y dio
        correcta: 15 y 15. **Eso es exactamente lo que esta fase aprendió a no hacer** — una
        comparación a mano es correcta hasta la migración que nadie re-compara. Se convirtió en
        `TestTenantIsolation_CoversEveryDeclaredTenantTable`, que falla en las dos direcciones
        (una tabla tenant sin caso; un caso sobre un nombre que la declaración no conoce) y
        detecta nombres duplicados. Mutación: **2/2**. Ese guard es ADR-0002's completion rule
        —*"a table without a passing case is not done"*— dejando de ser una frase.
      - **The count was "four" and is wrong** — noted at T-01-033 and corrected here rather than
        left for the task to discover. They are: LT-2 (shelter verification unenforced),
        `refresh_tokens`' access path, `app_public` on `shelters`, `pets.breed_id` allowing a
        breed of another species, B1 (privilege escalation within the tenant), assignment to a
        non-active member, and what survives a retention purge. A DoD that names a smaller
        number is satisfiable while leaving three decisions unmade.
      - est: 40
      - pilot: eligible
      - engram: —

---

## Phase exit

1. Every task above `[x]`.
2. The master plan's F01 verification run and recorded: for every table with `shelter_id`, a
   test proving tenant A cannot read or write tenant B's rows.
3. `go test ./...` green, coverage ≥ 75%, `test-race` green on CI, `make generate` no diff.
4. Engram queue empty or approved.
5. `/sdd-verify` green → `/sdd-archive`.

---

## Recovery note — 2026-08-29

Everything from T-01-006 onwards was **reconstructed** after I truncated this file. The close
script for T-01-005 spliced its result text in as `s[:i] + s[i:j] + new` and **omitted the
trailing `s[j:]`**, dropping tasks T-01-006 … T-01-035.

Recovered from the hybrid store's other half — Engram observation `#41`
(`sdd/phase-01-domain-and-data/tasks`), which carried the task index, every line estimate, the
non-negotiable tasks, the pilot blacklist, the parallelism pairs, the `pet_media` composite-key
decision, the open questions and the reported contradictions — plus `design.md`, the seven
capability specs and `docs/vault/30-fases/FASE-01.md`, all intact.

What is faithful: ids, titles, line estimates, PR assignment, blacklist marks, open questions,
and every decision closed since. What is reconstructed rather than restored: the per-task
`spec:` / `build:` / `tests:` / `dod:` wording for T-01-008 … T-01-035, rewritten from the same
authority documents the original was written from, and updated with the decisions taken since
(citext, the `media` policy moved to `00005`, `media`'s `UNIQUE (id, shelter_id)` in `00003`,
the PR re-balance).

The lesson worth keeping: the hybrid store is not bookkeeping. It is the only reason this was a
half-hour of rework instead of a lost afternoon.
