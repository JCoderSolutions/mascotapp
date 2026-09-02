# Design: Phase 01 — Domain and Data

> Implements **FASE-01**. Authority: plan §3/§4, ADR-0002, `proposal.md` (D1–D4 are
> settled and not reopened here). Mirror of Engram `sdd/phase-01-domain-and-data/design`.

## Technical Approach

One migration set, embedded once and replayed everywhere: `goose up` for the developer's
compose database, `goose.NewProvider(fsys)` for each ephemeral test container. Roles are
created by migration `00001`, before any table can enable RLS. Every table lands as one
indivisible unit — `CREATE TABLE` + `ENABLE`/`FORCE ROW LEVEL SECURITY` + policies +
explicit grants — and no table is considered done until its A/B isolation test is green.

The application reaches tenant data through exactly one door, `db.WithTenant`. Nothing
above `internal/db` ever sees the pool.

---

## Architecture Decisions

### D5 — Child tables carry a denormalised `shelter_id`, held true by a composite FK

**Choice.** The six tenant-scoped child tables — `pet_media`, `pet_health_records`,
`pet_status_history`, `form_template_versions`, `application_events`, `application_notes`
— each get `shelter_id uuid NOT NULL`, the same direct `tenant_isolation` policy as their
parent, and a **composite foreign key** to `parent (id, shelter_id)`. Each parent gains a
redundant `UNIQUE (id, shelter_id)` so the FK has something to reference.

```sql
ALTER TABLE pets ADD CONSTRAINT pets_id_shelter_key UNIQUE (id, shelter_id);

CREATE TABLE pet_status_history (
  id          uuid NOT NULL PRIMARY KEY,
  shelter_id  uuid NOT NULL REFERENCES shelters (id),
  pet_id      uuid NOT NULL,
  ...
  CONSTRAINT pet_status_history_pet_fk
    FOREIGN KEY (pet_id, shelter_id) REFERENCES pets (id, shelter_id) ON DELETE CASCADE
);
```

**Alternatives considered.** `EXISTS`-on-parent policy; denormalised column kept honest by
a `BEFORE INSERT OR UPDATE` trigger; denormalised column kept honest by application
discipline.

**Rationale — this is not a rubber stamp of the proposal's recommendation; the proposal's
own reasoning was incomplete.** "Direct and indexable" is the weakest of the four reasons.

1. **A denormalised column *without* the composite FK is actively worse than `EXISTS`.**
   PostgreSQL documents that referential integrity checks always bypass row security. With
   a plain `pet_id REFERENCES pets(id)`, tenant B can insert a `pet_media` row carrying
   B's own `shelter_id` and A's `pet_id`: the FK check bypasses RLS and passes, the
   `WITH CHECK` passes because `shelter_id` is B's, and B has just attached a row to
   another tenant's pet. `EXISTS`-on-parent would have rejected it. The composite FK
   closes this: `(A_pet_id, B_shelter_id)` is not a row in `pets`, so the constraint
   fails. **The composite FK is therefore mandatory, not an optimisation.**
2. **It closes a cross-tenant existence oracle.** A single-column FK answers "does this
   UUID exist in another tenant?" through the difference between success and violation.
   The composite FK returns the same violation for a foreign id and a nonexistent id.
3. **`EXISTS` couples a child's isolation to its parent's entire policy set.** Policies
   are permissive and OR-combined. `pets` will carry both `tenant_isolation` and the
   `public_catalog` policy for `app_public`. A sub-query inside a policy is evaluated as
   the querying role, so RLS on the referenced table applies too — meaning any policy
   added to a parent later silently widens every child. Two tables referencing each other
   also produce `infinite recursion detected in policy for relation`. A direct column
   comparison has no such coupling.
4. **Cost, stated honestly.** Per child table: 16 bytes per row, one extra unique index on
   the parent, and `shelter_id` written on every insert. On the read side, security quals
   are applied ahead of ordinary user quals, so an `EXISTS` becomes a sub-plan evaluated
   early on every candidate row of every query against the table. We accept a bounded,
   measurable write and storage cost to remove an unbounded and non-local read cost.

**Why the composite FK and not a trigger.** A trigger is per-row runtime code that a
future `ALTER TABLE ... DISABLE TRIGGER` removes; the FK is a declarative constraint the
planner and the catalog both know about, it costs an index probe the child's parent lookup
already needed, and it makes divergence *unrepresentable* rather than merely detected.
Application discipline is precisely the failure mode ADR-0002 exists to eliminate.

### D6 — The rule: denormalise where a row has exactly one shelter; `EXISTS` only where it has many

`users` genuinely belongs to N shelters and cannot carry a `shelter_id`. It is the single
sanctioned `EXISTS` carve-out, scoped through `memberships`.

> **`AND m.status = 'active'` added 2026-08-30 by T-01-016 (Judgment Day), and it is not
> cosmetic.** Without it, ANY membership row makes a user visible — and `app_tenant` holds
> INSERT and UPDATE on `memberships`. So a tenant could **mint read access to any user's
> PII** by inserting an `invited` membership for that user id, and revoking it did not take
> the visibility away. Reproduced live on PostgreSQL 17 before the fix.
>
> The general rule, worth carrying to every future `EXISTS` carve-out: **when a policy on
> table P derives visibility through bridge table B, WRITE permission on B is READ
> permission on P.** Ask who can write the bridge, and filter on the state the business
> actually means — here, the `active` this document's own spec already said.

```sql
CREATE POLICY member_visible_users ON users FOR SELECT TO app_tenant
  USING (EXISTS (SELECT 1 FROM memberships m
                 WHERE m.user_id = users.id
                   AND m.shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid
                   AND m.status = 'active'));
```

Migration `00009` adds a second permissive policy for the applicant path
(`adoption_applications.applicant_user_id`), because `adoption_applications` does not
exist yet at `00002`. `species` and `breeds` are global reference data:
`FOR SELECT USING (true)`, `SELECT` grant only. `refresh_tokens` is identity-domain and
**default-deny** in this phase — RLS on, no policy, no grant; Phase 02 owns its access path.

**`memberships` is tenant-scoped under the standard direct policy** (added 2026-08-29 after
`sdd-spec` correctly pointed out this rule left it unstated). A membership row joins ONE
user to ONE shelter, so by D6's own test — exactly one shelter — it denormalises rather than
using `EXISTS`. It is not an exception; naming it here only removes the need to re-derive it.

**Consequence for the catalog meta-test.** A policy on a table whose RLS is not enabled is
**inert**. `users` is protected *only* by the `EXISTS` policy above, so the meta-test must
assert `relrowsecurity` AND `relforcerowsecurity` on all 19 tables, non-tenant ones
included — not just on the tenant set. Exempting them would leave every user row readable
by every tenant while the policy still read as correct and the suite stayed green.

### D7 — `users.email` is `citext`, as §4 specifies

> **REVERSED 2026-08-29 by user decision.** The original D7 replaced §4's `CITEXT` with a
> `lower(email)` unique index, reasoning that "Neon's allow-list is unconfirmed". **That
> premise was false and has been verified false.** `citext` is not an allow-list gamble:
> Neon documents it on a dedicated page (`neon.com/docs/extensions/citext`), and
> `postgres:17-alpine` — the compose image — ships `citext 1.6` out of the box, confirmed by
> running `CREATE EXTENSION IF NOT EXISTS citext` in a throwaway container. Both target
> environments have it. With the premise gone, the deviation from §4 had nothing left
> holding it up.

`users.email` is **`citext NOT NULL UNIQUE`**, exactly as §4 writes it. Migration `00001`
runs `CREATE EXTENSION IF NOT EXISTS citext` as the owner role, before any table exists.

**Why this and not the `lower(email)` index.** Both enforce case-insensitive *uniqueness*.
They differ on *lookup*: with a `lower(email)` index, `WHERE email = $1` silently misses a
differently-cased address unless every caller remembers to normalise, on the write path and
the read path both. That is correctness resting on discipline — the exact failure mode
ADR-0002 exists to eliminate ("the database is the last line of defence and does not depend
on the discipline of whoever writes the SQL"). `citext` moves the property into the column
type, where it cannot be forgotten. The honest cost: each comparison lowercases internally,
so `citext` is measurably slower than `text`. On a login-by-email lookup that is noise.

**Scope of D3 is narrowed, not broken.** D3 rejected `pg_uuidv7`, which genuinely is not
available on Neon. It does not follow that *every* extension is unavailable. `citext` is
verified present in both environments; `pg_uuidv7` is not. UUIDv7 is still generated in Go.

**Rejected: `text` with a non-deterministic ICU collation.** PostgreSQL's modern answer, and
extension-free — but pattern-matching operators (`LIKE`, `~`) do not work on a
non-deterministic collation, which would quietly foreclose an admin user-search by prefix.

### D8 — Role passwords never appear in a migration

`ALTER ROLE ... PASSWORD` is a utility statement and cannot take bind parameters — the same
trap as `SET LOCAL`. Bootstrap runs outside goose, as owner:

```sql
SELECT set_config('app.bootstrap_password', $1, true);
DO $$ BEGIN EXECUTE format('ALTER ROLE app_tenant PASSWORD %L',
                           current_setting('app.bootstrap_password')); END $$;
```

The value arrives as a bind parameter; `format('%L')` does the quoting server-side. Note
that `log_statement = all` would still record the `DO` block's text, not the value.

### D9 — Grants are per-table, never `ON ALL TABLES`, never `ALTER DEFAULT PRIVILEGES`

A new table is unreachable by `app_tenant` until a migration grants it deliberately.
Default-deny is what makes the meta-test in §Testing able to fail loudly on an omission.

---

## The `WithTenant` contract

```go
// Beginner is the slice of *pgxpool.Pool that WithTenant needs.
type Beginner interface {
    Begin(ctx context.Context) (pgx.Tx, error)
}

// WithTenant runs fn inside a transaction whose tenant scope is set for the
// lifetime of that transaction and no longer. It is the only sanctioned path to
// tenant data: every RLS policy in this schema reads app.shelter_id, so a query
// issued outside this wrapper sees zero rows rather than the wrong rows.
func WithTenant(
    ctx context.Context,
    db Beginner,
    shelterID uuid.UUID,
    fn func(ctx context.Context, tx pgx.Tx) error,
) error
```

> **Contract refined 2026-08-29 during T-01-003.** The parameter was
> `pool *pgxpool.Pool`. A concrete type cannot be stubbed, so every guarantee below
> would only ever have been exercised by an integration test — meaning none of them
> would be exercised at all whenever the Docker daemon is down, which is exactly the
> condition under which someone is most tempted to "just ship it". Narrowing the
> parameter to the one method the wrapper actually calls costs nothing at the call
> site (`*pgxpool.Pool` satisfies `Beginner` as-is) and moves the whole contract into
> unit tests that need no container.
>
> This is not decoration. All six guarantees are now verified by mutation, not by
> coverage: removing the `uuid.Nil` check, flipping `set_config`'s `is_local` to
> `false`, swallowing the panic, dropping the rollback, running `fn` before the scope
> is set, and interpolating the tenant id instead of binding it — **each mutant was
> introduced and each one was killed by a failing test.**

Guarantees:

1. `Begin`, then `SELECT set_config('app.shelter_id', $1, true)`, then `fn`, then `Commit`;
   `Rollback` on any error and on panic (re-panicking after rollback).
2. `uuid.Nil` is rejected before `Begin` with `ErrNoTenant`.
3. `fn` receives only the `pgx.Tx`. The pool is never handed out.

**Why strictly inside `Begin`…`Commit`.** `pgxpool` hands out *physical* connections and
returns them to the pool for reuse. A session-scoped setting — plain `SET`, or
`set_config(..., false)` — survives the release. The next unrelated request to acquire that
connection inherits the previous tenant's scope, and every policy then evaluates correctly
against the wrong shelter. That is a silent cross-tenant read with no error anywhere. The
third argument `true` means *local to the current transaction*: PostgreSQL reverts the
setting at `COMMIT` or `ROLLBACK` as part of transaction teardown, so there is no cleanup
path left to forget or to skip on an early return.

`SET LOCAL` is **forbidden** — a utility statement, it cannot take bind parameters, so it
forces the tenant UUID to be interpolated into SQL text. The GUC is deliberately *not* set
in `pgxpool.Config.AfterConnect` or `BeforeAcquire` for the same reason.

`WithPublic(ctx, publicPool, fn)` is the `app_public` counterpart: `BEGIN READ ONLY`, no
GUC, separate pool, separate role.

sqlc `Queries` stay tenant-unaware and are bound per call via `sqlcgen.New(tx)`.

---

## File Changes

| File | Action | Description |
|---|---|---|
| `apps/api/go.mod` | Modify | `pgx/v5`, `pressly/goose/v3`, `testcontainers-go` + postgres module, `google/uuid` |
| `apps/api/sqlc.yaml` | Create | `sql_package: pgx/v5`, out `internal/db/sqlcgen`, `emit_interface: true`, uuid override → `google/uuid.UUID`. Co-located, mirroring `internal/api/config.yaml` |
| `apps/api/internal/db/db.go` | Create | `NewPool`, `NewPublicPool`, `Close`; pool sizing and `ConnConfig` |
| `apps/api/internal/db/tenant.go` | Create | `WithTenant`, `WithPublic`, `ErrNoTenant` |
| `apps/api/internal/db/migrate.go` | Create | `//go:embed migrations/*.sql`; `Migrations() fs.FS`; `Up(ctx, *sql.DB)` |
| `apps/api/internal/db/bootstrap.go` | Create | `SetRolePassword` (D8), owner connection only |
| `apps/api/internal/db/migrations/*.sql` | Create | 12 goose files, below |
| `apps/api/internal/db/query/*.sql` | Create | sqlc inputs, one file per aggregate |
| `apps/api/internal/db/sqlcgen/` | Create | generated, committed (D2) |
| `apps/api/internal/db/dbtest/` | Create | `container.go`, `roles.go`, `tenants.go` — importable helper package, not `_test` |
| `apps/api/internal/db/rlstest/` | Create | external `_test` package: the table-driven A/B suite and the catalog meta-test |
| `Makefile` | Modify | `migrate`, `migrate-down`, `db-reset`, `test-short`; `sqlc generate` in `generate` |
| `.github/workflows/ci.yml` | Modify | sqlc diff check; assert the RLS suite was not skipped |
| `.devcontainer/postCreate.sh` | Modify | install the `sqlc` CLI |
| `.env.example` | Modify | `DATABASE_URL_TENANT`, `DATABASE_URL_PUBLIC`, bootstrap password vars |

### Migration plan (dependency order)

| # | File | Contents |
|---|---|---|
| 00001 | `extensions_roles_and_grants.sql` | `CREATE EXTENSION IF NOT EXISTS citext` as owner (D7) — the **only** extension in this schema. Then `app_tenant`, `app_public` via `DO` guarded on `pg_roles`; `NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS LOGIN`, no password. `REVOKE ALL ON SCHEMA public FROM PUBLIC`, `GRANT USAGE` to both. **Down revokes grants, never drops roles** (D1b), and never drops the extension — dropping it would cascade the `users.email` column |
| 00002 | `tenancy_identity.sql` | `shelters`, `users`, `memberships`, `refresh_tokens` + policies (D6) |
| 00003 | `media.sql` | `media` + tenant policy + `UNIQUE (id, shelter_id)` for `documents`' composite FK. **No public policy here** — it depends on `pets` (`00005`) |
| 00004 | `reference_data.sql` | `species`, `breeds` + seeds, read-only policies |
| 00005 | `pets.sql` | `pets`, `UNIQUE (id, shelter_id)`, §4.6 indexes, tenant + `public_catalog` policies, **and `media`'s deferred public policy** (see the ordering correction above) |
| 00006 | `pet_children.sql` | `pet_media`, `pet_health_records`, `pet_status_history` (D5) |
| 00007 | `form_templates.sql` | `form_templates`, `form_template_versions` (D5) + published-version immutability trigger |
| 00008 | `form_submissions.sql` | `form_submissions` + GIN index on `answers` |
| 00009 | `adoption_applications.sql` | `adoption_applications` + index + the applicant `users` policy (D6) |
| 00010 | `application_children.sql` | `application_events` (append-only), `application_notes` (D5) |
| 00011 | `documents.sql` | `documents` |
| 00012 | `audit_log.sql` | `audit_log` (append-only, `BIGSERIAL`) + `(shelter_id, occurred_at DESC)` index + sequence grant |

Roles move from the proposal's `002` to `00001`. `citext` is created in that same file, ahead
of the roles, because `users.email` in `00002` depends on it and because creating an extension
requires the owner — the same privilege level the role creation already needs. `pg_uuidv7` is
still absent (D3): `citext` is verified available on Neon, `pg_uuidv7` is not.

**`Down` must not drop the extension.** `DROP EXTENSION citext` would cascade to
`users.email`, silently destroying a column that `down-to` is supposed to leave recoverable.
The extension is created `IF NOT EXISTS` and never dropped — the same asymmetry as roles (D1b),
and for the same reason: it may be shared by objects this migration does not own.

---

## The RLS policy template

Applied verbatim to every tenant-scoped table (`shelters` substitutes `id` for `shelter_id`):

> **CORRECTED 2026-08-30 by T-01-015, confirmed by T-01-016 (Judgment Day).** The
> template below said `current_setting('app.shelter_id', true)::uuid`. That form is
> **wrong on a pooled connection.** A GUC set inside a transaction and reverted at
> COMMIT or ROLLBACK does not go back to *unset* — it goes back to the **empty
> string** — so `shelter_id = ''::uuid` raises `22P02` instead of returning zero
> rows. It failed closed on a virgin connection and loudly on a reused one, and
> after the first request every pooled connection is a reused one. `nullif` makes
> the two states identical. Both halves are load-bearing: `missing_ok` covers
> never-set, `nullif` covers reverted-to-empty.

```sql
ALTER TABLE <t> ENABLE ROW LEVEL SECURITY;
ALTER TABLE <t> FORCE  ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON <t> FOR ALL TO app_tenant
  USING      (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid)
  WITH CHECK (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON <t> TO app_tenant;
```

- **`FORCE`** makes the policy apply to the table *owner* too, so a migration or admin
  connection cannot silently bypass it. It does **not** apply to superusers or `BYPASSRLS`
  roles — which is exactly why the `pg_roles` guard below is not optional.
- **`current_setting(..., true)`** — the `true` is `missing_ok`. An unset GUC yields `NULL`,
  the comparison yields `NULL`, and the query returns zero rows. Fail-closed by construction.
- **`WITH CHECK` is written explicitly** even though PostgreSQL would infer it from `USING`
  for a `FOR ALL` policy, because that inference vanishes the moment the policy is split
  into per-command policies — as it is for the append-only tables.
- **Public surface** (`shelters`, `species`, `breeds`, `pets`, `media`, `pet_media`) adds a
  role-scoped read-only policy and a `SELECT`-only grant:

```sql
CREATE POLICY public_catalog ON pets FOR SELECT TO app_public
  USING (status = 'available' AND published_at IS NOT NULL AND deleted_at IS NULL);
GRANT SELECT ON pets TO app_public;
```

  Permissive policies OR together, but `TO app_public` keeps that branch off `app_tenant`
  entirely. Role scoping is what makes the union safe. `media` is the one deliberate nested
  reference: it is public only through a published pet, and the nested `pets` lookup is
  itself filtered by `public_catalog`, which reinforces rather than widens it.

> **Ordering correction, 2026-08-29** (found by `sdd-tasks`; decided here rather than left
> to apply). `media`'s `public_catalog` policy depends on `pets`, but `media` is migration
> `00003` and `pets` is `00005` — the policy **cannot be created where this section implies**.
>
> **Decision: `00003` creates `media` with its tenant policy only. `00005` adds `media`'s
> public policy, immediately after creating `pets`.** A migration may add a policy to a table
> an earlier migration created; it may not reference a table that does not exist yet. The
> `app_public` A/B assertions for `media` therefore live with `pets`, not with `media`.
>
> **`media` also gets `UNIQUE (id, shelter_id)` in `00003`**, at creation. `documents`
> (`00011`) needs it for its composite FK on `(media_id, shelter_id)` per D5. Every parent in
> a composite tenant FK declares that key in its **own** migration — the same rule `pets`
> follows — so `00011` never reaches back to `ALTER` a table eight migrations upstream.

### Append-only: `application_events` and `audit_log`

Three independent layers, each covering a different attacker:

```sql
CREATE POLICY tenant_read   ON application_events FOR SELECT TO app_tenant USING (...);
CREATE POLICY tenant_append ON application_events FOR INSERT TO app_tenant WITH CHECK (...);
-- deliberately no UPDATE or DELETE policy

REVOKE UPDATE, DELETE, TRUNCATE ON application_events FROM app_tenant, PUBLIC;
GRANT  SELECT, INSERT             ON application_events TO   app_tenant;
```

**Is a trigger also needed, given the table owner?** Yes, and for a reason that is not the
obvious one.

- The **grant** stops `app_tenant`. The **absent policy** is defence in depth: even if a
  grant were restored by mistake, no permissive `UPDATE`/`DELETE` policy exists, so under
  `FORCE` the statement matches zero rows — including for the owner.
- Neither stops a `BYPASSRLS` role. On Neon that is not hypothetical: `neon_superuser`
  carries `BYPASSRLS`. **RLS is bypassed by such roles; triggers are not.** A
  `BEFORE UPDATE OR DELETE` trigger raising an exception is the only append-only
  enforcement that survives it.
- It also converts a *silent* zero-row result — which application code readily misreads as
  success — into a loud error. Add a `BEFORE TRUNCATE` statement trigger for the same reason.

The trigger is not a boundary against the owner (who can `DISABLE TRIGGER`); it is a
correctness signal and the last line under a mis-provisioned role. Both are worth having.

---

## Data Flow

```
HTTP handler (Phase 02) ── shelterID from the JWT claim, never URL or header
        │
        ▼
db.WithTenant(ctx, pool, shelterID, fn)
        │  1. pool.Begin                     → pgx.Tx on a pooled physical connection
        │  2. SELECT set_config('app.shelter_id', $1, true)
        ▼
      fn(ctx, tx) ── sqlcgen.New(tx).<Query>       [tenant-unaware by design]
        ▼
PostgreSQL  policy USING (shelter_id = current_setting('app.shelter_id', true)::uuid)
        │
        ▼  3. Commit / Rollback → GUC reverted with the transaction; connection returns clean
```

---

## Testing Strategy

RED-first order per table, which is coarser than usual because a policy cannot be asserted
without a database: **(1)** add the table's row to the A/B suite and to the meta-test's
tenant list → fails, the relation does not exist; **(2)** write the migration (table +
`ENABLE`/`FORCE` + policies + grants); **(3)** `sqlc generate`; **(4)** green. Do not split
finer — a table with no policy is not a meaningful assertion target.

| Layer | What to test | Approach |
|---|---|---|
| Unit | `WithTenant` rejects `uuid.Nil`; rolls back on `fn` error; rolls back and re-panics on panic; never commits after an error | `pgx.Tx` is an interface — a recording stub, no database, `t.Parallel()` |
| Unit | Embedded migrations: FS non-empty, versions strictly ascending with no gaps, every `Up` has a `Down` | pure Go over the embedded `fs.FS` |
| Integration | `goose up` → `down-to 0` → `up` again is clean and idempotent | testcontainers, fresh container |
| Integration | **Role guard (mandatory)** | `SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user` must return `(false, false)` for `app_tenant` and `app_public`, plus `current_user <> 'mascotapp'`. Asserted in harness setup so every table test inherits it |
| Integration | **A/B isolation, per tenant table** (ADR-0002 completion rule) | table-driven, below |
| Integration | **Catalog meta-test** | every table in `public` is either in the tenant list or in an explicit non-tenant allow-list; every tenant table has `relrowsecurity` **and** `relforcerowsecurity` true and ≥1 policy |
| Integration | Append-only: `UPDATE`/`DELETE`/`TRUNCATE` on `application_events` and `audit_log` fail | expect `42501`, or the trigger's exception |
| Integration | `app_public` sees only published, non-deleted, available pets and cannot write | separate pool and role |
| E2E | — | N/A this phase |

### A/B harness

```go
package dbtest

// Postgres starts one container per test package. It skips on -short because
// the Docker daemon is not guaranteed on the Windows development host.
func Postgres(t *testing.T) *Env

type Env struct {
    OwnerPool  *pgxpool.Pool // migrations and fixtures only
    TenantPool *pgxpool.Pool // connects as app_tenant
    PublicPool *pgxpool.Pool // connects as app_public
    ShelterA, ShelterB uuid.UUID
}
```

Per table, as `app_tenant` throughout:

1. `WithTenant(A)`: insert row R.
2. `WithTenant(B)`: `SELECT` → 0 rows; `UPDATE` → 0 rows affected; `DELETE` → 0 rows
   affected; `INSERT` carrying A's `shelter_id` → `WITH CHECK` violation (`42501`).
3. **Child tables additionally**: as B, insert a child row referencing A's parent id →
   foreign key violation (`23503`). This is the assertion that proves D5's composite FK
   closes the cross-tenant orphan path that a single-column FK leaves open.
4. `WithTenant(A)`: R is still present and unmodified.

Adding a table means adding one struct literal. The meta-test is what makes forgetting to
do so a build failure rather than a silent gap.

**Docker gating.** `Postgres` skips on `testing.Short()`. When the daemon is simply
unreachable it **fails** rather than skips, unless `MASCOTAPP_SKIP_DOCKER_TESTS=1` is set —
and `TestMain` fails outright if that variable is set while `CI=true`. A suite that can
silently skip itself is a suite that rots. `make test-short` is the Windows-host path;
`make test-api` and CI run the full suite.

---

## Threat Matrix

`N/A` — this change introduces no routing, shell command, VCS/PR automation, or
executable-file classification boundary. The one subprocess-adjacent surface is
`testcontainers-go` talking to the Docker daemon socket; it is test-only, its inputs are
compile-time constants rather than user data, and it never runs in a production binary.

---

## Migration / Rollout

Per `proposal.md`'s rollback plan; unchanged. Phase 01 is greenfield, so `goose down-to`
is schema-only with zero data loss — a property that expires the moment Phase 02 ships.
`make db-reset` owns local teardown (the Makefile owns every deletion).

---

## Promote to ADR (`docs/vault/20-arquitectura/`) — named, not written here

> Numbering corrected 2026-08-29: ADR-0001..0006 are already taken
> (`stack`, `multi-tenancy`, `costo-cero`, `tls-sin-mtls`, `opencode-piloto`,
> `modo-permisos`). This phase's ADRs start at **0007**.

| ADR | Subject |
|---|---|
| ADR-0007 | Tenant denormalisation on child tables, enforced by composite tenant foreign keys (D5/D6) |
| ADR-0008 | `WithTenant` as the only path to tenant data: transaction-scoped GUC, no pool-level binding |
| ADR-0009 | Roles, grants and passwords provisioned only by migration; default-deny for new tables (D8/D9) |
| ADR-0010 | Append-only tables: grants, absent policies and triggers, and why all three are needed |
| ADR-0011 | Extension policy: UUIDv7 generated in Go because `pg_uuidv7` is unavailable on Neon, while `citext` is used because it is verified present in both environments (D3/D7) |

---

## Open Questions

- [x] ~~D7 replaces §4's `CITEXT` with a `lower(email)` unique index.~~ **RESOLVED
      2026-08-29:** premise verified false, D7 reversed, `citext` restored per §4.
- [ ] `refresh_tokens` is default-deny in this phase. Phase 02 must decide its access path:
      a dedicated `app_auth` role, or a user-scoped `app.user_id` GUC alongside the tenant one.
- [ ] `users` gets `SELECT` only for `app_tenant` in Phase 01. Who may write a `users` row,
      and through which role, belongs to Phase 02's registration flow.
- [ ] Whether `pet_status_history` should also be append-only. §4 does not say; it is
      historically shaped and would be cheap to lock down now, expensive later.
- [ ] The three unanswered questions in `proposal.md` (refresh tokens now vs. Phase 02,
      4 chained PRs vs. one `size:exception`, `app_public` in 01B vs. Phase 06) still stand.
