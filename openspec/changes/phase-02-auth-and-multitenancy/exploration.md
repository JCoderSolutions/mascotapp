# Exploration — phase-02-auth-and-multitenancy

- **Date:** 2026-09-01
- **Phase:** 02 — Auth and multi-tenancy
- **Store:** hybrid. Mirror in Engram at `sdd/phase-02-auth-and-multitenancy/explore` (obs `#151`).

> **Provenance.** Produced by `sdd-explore`, which is investigation-only and cannot write project
> files; this file was written by the orchestrator from that result. Its load-bearing claims were
> spot-checked against source before being recorded — see *Verification of this report* at the end.

## Executive summary

Phase 02 starts from **zero HTTP-auth code**, on top of a Phase 01 database layer that already
anticipates it: nullable `password_hash`, a `refresh_tokens` table with the rotation columns, a
client-IP resolver built for rate limiting, and a role-name pattern that already accepts a future
`app_auth`.

What it does **not** start from is a clean slate on decisions. Three are open **in code**, each
pinned by a test deliberately written to go red the day it is resolved.

## 1. What already exists

| Area | Where | Note |
|---|---|---|
| The only DB door | `apps/api/internal/db/tenant.go` | `WithTenant` / `WithPublic`. `set_config(..., true)`, transaction-local, never on the pool. `ErrNoTenant` refuses `uuid.Nil`. Rollback on error/panic, panic re-raised. |
| Identity tables | `internal/db/migrations/00002_tenancy_identity.sql` | `users` (citext email, **nullable** `password_hash`, `totp_secret_enc bytea`, `status`), `memberships` (role enum, `UNIQUE (user_id, shelter_id)`), `refresh_tokens` (`token_hash`, `family_id`, `expires_at`, `revoked_at`, `replaced_by`, `ip_hash`) — RLS on, **zero policies, zero grants**. |
| Roles | `00001_extensions_roles_and_grants.sql`, `bootstrap.go` | `app_tenant`/`app_public` created `NOBYPASSRLS` in SQL only. `roleNamePattern` = `^app_[a-z][a-z0-9_]{0,40}$` — **already accepts `app_auth` unchanged**. |
| Client IP | `internal/httpapi/clientip.go` | Rightmost-untrusted-entry scan. Its own comment names *"per-IP rate limiting on login and magic-link endpoints"* as the reason it exists. `ClientIPFromContext` is ready to be read by a limiter. |
| HTTP | `internal/httpapi/router.go` | chi + RequestID, Recoverer, ClientIP, JSON request logger. Routes: `/healthz`, `/readyz`. **Nothing else.** |
| Decisions | `docs/vault/20-arquitectura/ADR-0007…0011` | ADR-0009 names *"Fase 02 necesita un tercer rol (`app_auth`)"* and the B1 column-grant fix as its own reopening conditions #1 and #3. |
| Specs | `openspec/specs/` | Seven capabilities, merged at archive. `tenant-isolation` already codifies `refresh_tokens` as default-deny, with a named scenario. |
| Query conventions | `internal/db/query_test.go` | `TestQueries_DoNotFilterByShelterID`; and `TestQueries_ExerciseEveryDeclaredTable` exempts `refresh_tokens` **via `rlstest.Schema.NoPolicy`** — derived, not hardcoded, so **the exemption self-expires** the day this phase grants the table. |

## 2. What is genuinely missing

- **The entire HTTP auth layer.** No login / register / magic-link / refresh / TOTP handlers, no
  JWT verification middleware, no claim types, no RBAC permission-matrix code.
- **No libraries wired**: no `golang-jwt`, no TOTP library, no direct Argon2. `golang.org/x/crypto`
  is present only as an **indirect** transitive dependency of the container tooling.
- **No auth surface in `api/openapi.yaml`** — the single API contract has none of it.
- **No email-delivery adapter.** Resend is named in the plan's cost table; there is no client and
  no confirmed config key.
- **No rate limiter.** The IP-resolution building block exists; the limiter does not.
- **No access path to `refresh_tokens`** — unreachable by any application role today.

## 3. Where the three inherited decisions bite, with the line that forces each

### 3.1 `refresh_tokens` access path

`00002_tenancy_identity.sql` enables RLS and states *"Deliberately no policy on refresh_tokens"*.
The grant block grants `shelters` / `memberships` / `users`, and for this table carries only a
**comment** — `-- refresh_tokens gets no grant at all.` — with **no SQL statement following it**.
Verified: no `GRANT` or `REVOKE` in that migration names the table.

`TestRefreshTokens_RefuseAppTenantEntirely` proves read *and* write fail under a **valid** tenant
scope, so the refusal is **grant-level, not scope-level**.

The choice ADR-0009 names: a dedicated `app_auth` role, or an `app.user_id` GUC alongside the
tenant one. **`WithTenant` takes `shelterID uuid.UUID` only**, so a user-scoped path needs either a
parallel wrapper or an extension of that contract — which is itself an ADR-0008 decision point.

### 3.2 B1 — privilege escalation within the tenant

`00002_tenancy_identity.sql` grants `SELECT, INSERT, UPDATE, DELETE` on `shelters` and on
`memberships` at **table granularity, no column restriction**. ADR-0009 records the live
confirmation: a tenant can set `shelters.status = 'verified'` (**self-verifying, bypassing LT-2**),
raise its own `storage_quota_bytes`, and set `memberships.role = 'owner'`.

**The trip-wire is real and cuts both ways.** `TestApplicantPolicy_IsAsWideAsWritingAnApplication`
currently pins the **wide** state and is designed to go red when column-level grants land — so
landing the fix **breaks that test on purpose**. Updating it is part of the same change, **not a
regression**.

### 3.3 Assignee / member-visibility gap

`00009_adoption_applications.sql`'s `adoption_applications_assignee_fkey` references
`memberships (user_id, shelter_id)` — the plain `UNIQUE`, not a partial one on `status = 'active'`
— because **a foreign key cannot reference a partial unique index** (verified on PG 17). So the key
proves *the pair exists*, not *is active*, while `member_visible_users` filters on `active`.

`TestAssignment_DoesNotYetRequireAnActiveMembership` pins the current, wider behaviour. **No
`UNIQUE (user_id, shelter_id) WHERE status = 'active'` exists to reference even if wanted.**

## 4. Tensions between the plan and what Phase 01 built

**An `app.user_id` GUC would break ADR-0008's single-GUC contract.** Every existing policy reads
only `app.shelter_id`. A second concurrent GUC is a real architectural addition, not a drop-in, and
*"the only door"* would have to become two doors or a `WithUser` coexisting with `WithTenant`. The
alternative — a dedicated `app_auth` role — leaves `WithTenant` untouched **but means refresh-token
queries run under a role with no shelter awareness at all**. The plan does not anticipate this
trade-off.

**"RBAC evaluated in the domain" (§5.3) sits awkwardly next to B1 still being open.** The plan
assumes RBAC layers on top of a database that already enforces column-level boundaries. Today the
grants are table-wide, so a domain-only check for *"can this role verify a shelter"* would be the
**only** thing preventing self-verification — **there is no database backstop**. And there is a
genuine contradiction between two documents: ADR-0009 defers B1 to *"Fase 02/03… endpoints que Fase
03 todavía no escribió"*, while `FASE-02.md`'s inherited-scope section reads as if **this** phase
owns it.

**Magic link needs `users.password_hash` nullable** — already true. But there is no email delivery
in code, and **no TOTP library or secret-format decision recorded anywhere**.

## 5. Open questions a proposal must answer

1. `refresh_tokens` access path: `app_auth` role vs `app.user_id` GUC — and its interaction with
   `WithTenant` / `WithPublic`.
2. Does Phase 02 land the B1 column-level grants, or formally re-defer to Phase 03? *(Genuine
   document contradiction — needs an explicit decision, not silent interpretation.)*
3. Assignee/member-visibility: trigger or domain check? If domain, RBAC needs read access to
   `memberships.status` at authorization time, which has its own tenant-scoping implications.
4. TOTP secret format, library, and recovery-code strategy — undecided anywhere.
5. Refresh-token cookie mechanics: does the API set `Set-Cookie` itself? That implies a same-origin
   or CORS-with-credentials design, and interacts with the §5.1 CORS allowlist, not yet implemented.
6. Rate limiting: in-memory per instance, or a shared store? **Nothing in the free-tier table names
   a backend** — no Redis, no KV.
7. Email delivery: is wiring Resend in scope, or does it land as an interface plus a stub?

## Verification of this report

Spot-checked by the orchestrator against source before recording:

- `roleNamePattern` at `bootstrap.go:69` is `^app_[a-z][a-z0-9_]{0,40}$` — accepts `app_auth`. ✅
- No `GRANT`/`REVOKE` in `00002` names `refresh_tokens`; only the comment at line 269. ✅
- `go.mod` has no `golang-jwt`, no `pquerna/otp`, no direct `argon2`. ✅
- **Corrected:** the report claimed *zero* matches for auth terms in `api/openapi.yaml`; there are
  **two**, and both are false positives on the word *"registered"*. **The conclusion stands** — the
  contract has no auth surface.

## Risks carried into the proposal

- **`.env.example` could not be read** (denied to agents by a global settings rule), so the presence
  of `RESEND_API_KEY` / `JWT_SECRET` is **unverified, not confirmed absent**. Do not assume either
  way; ask the user or have them paste the contents.
- **Two fixes intentionally break pinned tests.** The proposal must carry updating them as in-scope
  work.
- **ADR-0009 and `FASE-02.md` disagree on who owns B1.** Decide explicitly.
