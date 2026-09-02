# Proposal: Phase 02 — Auth and multi-tenancy

Implements **FASE-02**. Baseline: `exploration.md` in this folder.

## Intent

Phase 01 built a database that refuses anonymous access and cannot leak across tenants, but nothing
can reach it: there is zero HTTP auth code, `refresh_tokens` is unreachable by any role, and the
`shelter_id` every policy depends on has no producer. This phase builds the producer — identity,
session, and the token claim that opens `WithTenant` — and closes the two Phase 01 findings that
have no database backstop today.

## Scope

### In scope

- Register / login (Argon2id), magic link for adopters, TOTP for `owner`/`admin` (§5.2).
- JWT access token (15 min) + rotating refresh with reuse detection revoking the whole `family_id`.
- Access path for `refresh_tokens`: it stops being default-deny and gains **both** a grant and a policy.
- Tenant middleware: `shelter_id` from the token claim only, then `WithTenant`.
- RBAC permission matrix by `membership.role`, evaluated in the domain (§5.3).
- **B1 column-level grants** on `shelters` and `memberships` — see Decisions.
- In-memory per-instance rate limiting on login and magic link, over the existing `ClientIPFromContext`.
- **CORS allowlist with credentials + explicit CSRF protection** (§5.1), pulled INTO this phase by
  the separate-origins decision — see Decisions 4.
- **TOTP recovery codes**, including the new table and its grant, policy and catalog classification.
- Email **port + logging stub**.
- Auth surface added to `api/openapi.yaml`.
- Updating the two tests written to go red here: `TestApplicantPolicy_IsAsWideAsWritingAnApplication`
  and `TestAssignment_DoesNotYetRequireAnActiveMembership`. **This is scope, not regression.**

### Out of scope

| Deferred to | What |
|---|---|
| Phase 03 | Shelter CRUD, invitations — the endpoints that decide which columns a tenant writes |
| Phase 06 | LT-2 shelter verification, `app_public` grants on `shelters` |
| Phase 11 | Hardening; a rate limiter that survives a cold start |
| Later, conditional | Returning the refresh cookie to `SameSite=Strict` and retiring the CSRF token, **once** web and API share one hostname — see Decisions 4 |
| Unassigned | Wiring the real Resend client. No domain is registered; the stub keeps the flow testable |

## Decisions already taken (do not reopen)

1. **B1 is closed here.** This is the phase that builds RBAC. Without column grants, a domain check
   would be the *only* thing stopping a shelter from setting `status = 'verified'` and bypassing
   LT-2 — no database backstop, which is precisely what ADR-0002 exists to prevent. This resolves
   the contradiction between ADR-0009 (defers to 02/03) and FASE-02 (frames 02 as owner):
   **Phase 02 owns it, and ADR-0009's reopening condition #3 is discharged, not re-deferred.**
2. **Email is an interface plus a stub.** The auth flow stays end-to-end testable with no external
   account, key, or network.
3. **Rate limiting is in-memory and per instance.** No free-tier backend is named, and Postgres
   bills by CU-hour. **Stated limitation:** the counter resets on every cold start and does not
   coordinate across instances, so it stops clumsy brute force and not a patient attacker.
   Revisit when a shared store exists for another reason, or when an incident shows it matters.

> **Amended 2026-09-01, after the proposal question round.** Four items below moved from open to
> decided. They are recorded here rather than left to design because each one changes what this
> phase BUILDS, not how it builds it.

4. **Deployment is SEPARATE ORIGINS, so CORS and CSRF are Phase 02 work.** The user will register a
   domain, but **not before the first MVP deployment** — so the first deploy lands on `*.pages.dev`
   and `*.run.app`, which are different sites (different eTLD+1).
   **Consequence, and it is not small:** `SameSite=Strict` as §5.2 writes it **cannot hold** — the
   browser would not send the cookie and refresh would fail silently, as a logout every 15 minutes
   that reads like a session bug. This phase therefore builds `SameSite=None; Secure`, the §5.1 CORS
   allowlist **with credentials** (so never `*`), and **explicit CSRF protection** — the defence
   `Strict` would have given for free.
   **Revisit condition, stated so it does not rot:** once web and API are behind one hostname, the
   cookie can return to `Strict` and the CSRF token can be retired. That is a deliberate later
   change, and the risk that it never happens is accepted knowingly.
5. **TOTP recovery codes are IN.** TOTP is mandatory for `owner`/`admin`, so without them a lost
   phone locks the shelter owner out with no way back. This **adds a table**, which means its own
   grant, its own policy, and its own entry in the tenant-isolation catalog set — the meta-test
   fails loudly otherwise, which is the design working.
6. **Shelter registration is OPEN, and a new shelter lands `pending_verification` with no forward
   path until Phase 06.** That is LT-2's mitigation working rather than a gap: nobody publishes
   until a human enables them. Until Phase 06 lands the condition and Phase 10 the interface,
   verification is a manual `UPDATE` by the operator.
7. **The magic-link endpoint is INDISTINGUISHABLE for a known and an unknown address** — same
   response, same timing; the mail goes out only if the account exists. Otherwise the endpoint is an
   existence oracle over email addresses, which is the same class of leak this schema closed three
   times in Phase 01 (microchip, cover photo, version key).

## Security properties and the layer that owns each

A property nobody assigned to a layer is a property nobody has.

| Property | Owning layer | Backstop |
|---|---|---|
| `shelter_id` originates only from a validated claim | Token verification middleware | `WithTenant` refuses `uuid.Nil` |
| No cross-tenant read or write | Postgres RLS | unchanged from Phase 01 |
| A shelter cannot self-verify (`shelters.status`) | **Column grant** | domain RBAC |
| A member cannot self-promote to `owner` | **Column grant** on `memberships.role` | domain RBAC |
| Quota cannot be self-raised | **Column grant** on `storage_quota_bytes` | domain RBAC |
| Tenant sessions cannot reach `refresh_tokens` | Grant to the auth role only | row policy |
| Stolen refresh token is contained | Rotation + `family_id` revocation | reuse detection |
| Second factor on privileged roles | Domain auth service | — |
| Authorization by role | Domain permission matrix | column grants above |
| Assignee is a real member of the shelter | Composite FK (exists) | active-check layer — open, below |
| Brute-force resistance | HTTP limiter, in-memory | **none — accepted, see Decisions** |

## Approach

Additive. `WithTenant` is not modified, so no rollback can regress tenant isolation.

**Recommended direction, with the predicate handed to design:** a dedicated **`app_auth` role with its
own door**, not an `app.user_id` GUC alongside the tenant one. Every existing policy reads only
`app.shelter_id`; a second concurrent GUC makes that uniformity a special case and forces every
policy to be re-reasoned. ADR-0009 reopening condition #1 already anticipates `app_auth` ("suma un
rol, no cambia la regla") and `roleNamePattern` accepts it unchanged. One door, one GUC each.

**Constraint design must solve, found while writing this:** refresh-token rotation looks the row up
**by `token_hash`, before the user is known**, so a user-scoped policy predicate cannot be set from
an identity we do not yet have. Either the presented token carries its `user_id` (the hash still
authenticates; the GUC only narrows), or the policy is role-scoped. Design decides — but a grant
without a policy is the widest possible state and is not an option.

## Capabilities

### New

- `identity-and-session`: registration, login, magic link, TOTP, refresh rotation and reuse detection.
- `authorization-rbac`: the permission matrix, and the tenant claim → `WithTenant` path.
- `auth-rate-limiting`: per-IP and per-account limits, with the per-instance limitation specified.
- `email-delivery`: the sending port and its stub.

### Modified

- `tenant-isolation`: `refresh_tokens` loses its declared no-policy exception; `app_auth` joins the
  role guard; grants on `shelters` / `memberships` narrow to column granularity.
- `data-model-core`: **only if** recovery codes need a table. Any new table also amends the
  tenant-isolation catalog set, or the meta-test fails loudly — which is the design working.
- `adoption-flow-data`: **only if** the assignee active-check lands in the database.

## Open — design owns these

| Question | Note |
|---|---|
| `refresh_tokens` policy predicate | Direction recommended above; predicate open |
| Assignee active-membership check | RBAC must already read `memberships.status` to grant a role at all, so the domain check is nearly free; a constraint trigger is the backstop. A partial-unique FK is **not available** on PG 17 |
| TOTP library and secret format | `totp_secret_enc bytea` implies encryption at rest. **Recovery codes are DECIDED IN — see Decisions 5**; their table shape is design's |
| ~~Refresh cookie mechanics~~ | **DECIDED 2026-09-01 — separate origins. See Decisions 4.** Design owns the mechanics (`None; Secure`, allowlist shape, CSRF strategy), not the topology |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Column grants written against Phase 03 endpoints that do not exist | High | Grant the **minimum** now; Phase 03 widens by migration, visible in a diff |
| Two pinned tests go red on purpose and read as breakage | High | Update each in the **same commit** as the change requiring it |
| A new role or table silently skips its grant/policy | Med | ADR-0009 default-deny plus the catalog meta-test already fail loudly |
| Rate limiter creates false confidence | Med | Documented as partial in the spec text, not only in code |
| `RESEND_API_KEY` / `JWT_SECRET` presence in `.env.example` is **unverified** | Med | `.env.example` is denied to agents. User confirms before apply |
| 400-line review budget exceeded | High | Slice: (a) `app_auth` + refresh access, (b) column grants + test updates, (c) auth HTTP surface, (d) RBAC matrix, (e) limiter + email stub |

## Rollback plan

- **Migrations** carry `-- +goose Down`. Reverting restores table-level grants and the
  `refresh_tokens` default-deny state. Ordering hazard: revoke `app_auth`'s grants **before**
  dropping the role, or the down migration fails.
- **Code** is additive — auth routes register in their own group; removing the registration removes
  the surface without touching Phase 01. `WithTenant` is untouched by design.
- **Pinned tests** are reverted by the same `git revert` that reverts their cause, because each pair
  ships in one commit.
- **Accepted cost:** rolling back invalidates every issued refresh token. All sessions end.

## Dependencies

- Phase 01, merged. New direct modules: `golang-jwt/v5`, a TOTP library, `golang.org/x/crypto`
  (currently indirect only) for Argon2id.
- **User confirmation** of `.env.example` contents.
- Test runner is `make test-api-container`. `openspec/config.yaml` still records a bare `go test`,
  which does not run on this host — carry the container runner into tasks and apply.

## Success criteria

- [ ] A forged `shelter_id` in a URL or header returns 403 (plan F02).
- [ ] Refresh-token reuse revokes the entire family, proven by test.
- [ ] `app_tenant` setting `shelters.status`, `storage_quota_bytes`, or `memberships.role` fails with `42501`.
- [ ] `refresh_tokens` is reachable through the auth door and still refused to `app_tenant`.
- [ ] The RBAC matrix test covers every role × permission cell.
- [ ] `owner` / `admin` cannot complete login without TOTP.
- [ ] `TestQueries_DoNotFilterByShelterID` and the catalog meta-tests stay green.
- [ ] `make test-api-container` green; coverage ≥ 75%.
