# Design: Phase 02 — Auth and multi-tenancy

> Implements **FASE-02**. Authority: `proposal.md` (Decisions 1–7 are settled and not reopened),
> `exploration.md`, ADR-0002, ADR-0007…ADR-0011, plan §5.1–§5.4. Mirror of Engram
> `sdd/phase-02-auth-and-multitenancy/design`.
>
> **Decision numbering is phase-local and prefixed `P2-Dn`.** Phase 01's `D5`…`D9` are a different
> series and are referenced by their own names.
>
> **Every load-bearing fact below is marked `[verified]` or `[assumed]`.** Phase 01 repeatedly found
> that an inherited assumption nobody re-checked had already bent the design.

## Technical Approach

Additive, in four migrations (`00013`–`00016`) and one new HTTP layer. `WithTenant` is not modified
— its six mutation-tested guarantees keep their exact shape. The auth domain gets its **own role,
its own pool, its own door and its own GUC**, against a grant set that is disjoint from
`app_tenant`'s. Nothing an existing policy reads changes meaning.

The producer of `shelter_id` lands at the top: token verification puts a validated claim in the
request context, and the tenant middleware hands that value — and only that value — to `WithTenant`.

---

## Architecture Decisions

### P2-D1 — A third door, not a second GUC on the first: `app_auth` + `WithAuthUser` / `WithAuthLookup`

**Choice.** Migration `00013` creates `app_auth` (`NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS
NOINHERIT LOGIN`, guarded on `pg_roles` exactly like `00001`) `[verified: bootstrap.go:69
roleNamePattern ^app_[a-z][a-z0-9_]{0,40}$ accepts it unchanged]`. `internal/db/auth.go` adds two
wrappers on a third pool:

```go
// WithAuthUser runs fn inside a transaction scoped to ONE user. It is the only
// path that may write identity data. uuid.Nil is refused: an unscoped write
// would match a policy that is false, silently affecting zero rows.
func WithAuthUser(ctx context.Context, db Beginner, userID uuid.UUID,
    fn func(ctx context.Context, tx pgx.Tx) error) error

// WithAuthLookup runs fn in a READ ONLY transaction with NO user scope, for the
// one query that cannot have one: finding a user by email before anybody is
// authenticated.
func WithAuthLookup(ctx context.Context, db TxBeginner,
    fn func(ctx context.Context, tx pgx.Tx) error) error
```

`WithAuthUser` sets `SELECT set_config('app.user_id', $1, true)` — the same statement shape as
`setScopeSQL`, for the same two reasons ADR-0008 gives: `set_config` binds, `SET LOCAL` cannot; the
third argument `true` means the scope dies with the transaction rather than riding a pooled
connection to the next request. Every `app_auth` policy reads
`nullif(current_setting('app.user_id', true), '')::uuid` — **`nullif` is mandatory, not defensive:
a reverted GUC returns the EMPTY STRING, not NULL** `[verified: 00002 lines 166–185, found live by
T-01-015]`.

**Alternatives considered.** (a) An `app.user_id` GUC set by `WithTenant` alongside `app.shelter_id`.
(b) A role-scoped policy on `refresh_tokens` with no user predicate. (c) A `SECURITY DEFINER`
function owning the whole refresh path.

**Rationale, and this is an amendment to ADR-0008, argued rather than assumed.** ADR-0008 says
`WithTenant` is the only door. Its own reopening condition #1 anticipates this shape — *"un camino
de acceso que legítimamente no sea por tenant… necesita su propio rol y su propia puerta, no una
excepción a ésta"* — and #2 names `refresh_tokens` by name `[verified: ADR-0008 lines 87–93]`. A
third door is safer than widening the first for a reason that is mechanical, not stylistic:

- **The two GUCs are never concurrent.** They live on different pools, different roles and different
  transactions. No connection ever carries both, so no existing policy is ever evaluated in a
  session where a second GUC exists. Option (a) makes that false everywhere at once: every one of the
  16 policies now runs beside a variable it does not read, and any future policy could read it by
  accident. That is the uniformity the proposal was protecting.
- **The grant sets are disjoint, and that is what carries the isolation.** `app_tenant` never gets a
  grant on `refresh_tokens` or `totp_recovery_codes`; `app_auth` never gets one on any tenant table.
  A bug in one layer cannot reach the other's rows even with the right GUC, because the privilege is
  absent — which is grant-level refusal, the same thing that makes
  `TestRefreshTokens_RefuseAppTenantEntirely` pass today `[verified]`.
- **`WithTenant` keeps its exact signature**, so every mutation-killed guarantee from T-01-003 stays
  killed. Rollback cannot regress tenant isolation because nothing about tenant isolation moved.

**Enforced by a new meta-test, because "they never mix" is a claim about future migrations too:**
`TestPolicies_DoNotCrossGUCs` scans `pg_policy` via `pg_get_expr` and fails if any policy applying
to `app_tenant` mentions `app.user_id`, or any policy applying to `app_auth` mentions
`app.shelter_id`. A separation nothing observes is not a separation.

**Cost, stated honestly.** A third pool (`maxConns = 4` per process, so instances × 12 connections
against a small Neon compute — the ceiling worth watching), a third DSN, a third bootstrap password,
and a third entry in the `dbtest` harness and its role guard. And **`app.user_id` is self-asserted by
the auth service**, exactly as `app.shelter_id` is self-asserted from a claim: the GUC is not
authentication, it is the layer that turns a forgotten `WHERE` into zero rows instead of every row.

### P2-D2 — `refresh_tokens` is user-scoped, and the presented token carries its own `user_id`

**Choice.** The refresh cookie's value is `<base64url(user_id)>.<base64url(secret)>`, 32 random bytes
of secret. `token_hash` stores `sha256(secret)`. Rotation parses the prefix, opens
`WithAuthUser(parsedUserID)`, and looks the row up by `token_hash` inside that scope.

```sql
CREATE POLICY auth_own_sessions ON refresh_tokens FOR ALL TO app_auth
    USING      (user_id = nullif(current_setting('app.user_id', true), '')::uuid)
    WITH CHECK (user_id = nullif(current_setting('app.user_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON refresh_tokens TO app_auth;
```

**The constraint this resolves.** Rotation looks the row up by `token_hash`, *before* the user is
known — so a user-scoped predicate cannot come from an identity we do not yet have. Making the
client present the `user_id` supplies it. **The hash still authenticates; the GUC only narrows.** A
client that lies about the prefix scopes itself to a user whose rows do not contain that hash, gets
zero rows, and is refused — it gains nothing, because a valid session still requires the secret.

**Alternatives considered.** (a) `FOR ALL TO app_auth USING (true)` — role-scoped only. (b) No policy
at all with a grant — **explicitly not an option**: Phase 01 established that a grant without a
policy is the widest state reachable, and `refresh_tokens` leaves `rlstest.Schema.NoPolicy` here, so
`CheckProtection` would fail it `[verified: catalog.go:353]`. (c) Keep the lookup unscoped in a
`WithAuthLookup` transaction and scope only the writes.

**Rationale.** (a) buys nothing over the grant it sits behind: a forgotten `WHERE user_id = $1` in
any session query returns *every session on the platform*, which is precisely the class of bug
ADR-0002 layer 3 exists to convert into zero rows. Under this policy the same forgotten clause
returns exactly that user's sessions — which is what the query wanted anyway. (c) splits the read
and the write of one rotation across two transactions and loses atomicity on the reuse-detection
path, where the whole point is that detection and family revocation are one statement's distance
apart.

**Reuse detection under this predicate.** A presented token whose row has `revoked_at IS NOT NULL`
triggers `UPDATE refresh_tokens SET revoked_at = now() WHERE family_id = $1 AND revoked_at IS NULL`
— every row in a family shares one `user_id`, so the whole family is inside the scope already.
**Honest limitation:** an attacker who mangles the `user_id` prefix is not detected as reuse. They
also get no session, so nothing is lost by not alarming on an attack that already failed.

**What this path does not cover.** A global sweep of expired rows (`WHERE expires_at < now()`) cannot
run under `app_auth`, because it spans users. Expired rows are simply never matched by the rotation
predicate; retention/purge is Phase 11's, run as owner or through its own door. Stated so it is not
discovered later as a leak.

### P2-D3 — What else `app_auth` reaches: wide read on `users`, narrow columns; user-scoped read on `memberships`

Login must find a user by email before anyone is authenticated, and must list the user's shelters
before a `shelter_id` exists. Neither question is answerable under `app_tenant`: `users` is visible
only through `member_visible_users` / `applicant_visible_users`, and `memberships`' policy needs the
tenant GUC we are trying to derive `[verified: 00002:245, 00009:142]`.

```sql
CREATE POLICY auth_lookup_users ON users FOR SELECT TO app_auth USING (true);
CREATE POLICY auth_own_user     ON users FOR UPDATE TO app_auth
    USING      (id = nullif(current_setting('app.user_id', true), '')::uuid)
    WITH CHECK (id = nullif(current_setting('app.user_id', true), '')::uuid);
CREATE POLICY auth_register_user ON users FOR INSERT TO app_auth WITH CHECK (true);

GRANT SELECT (id, email, password_hash, status, totp_secret_enc, email_verified_at, created_at)
    ON users TO app_auth;
GRANT INSERT (id, email, password_hash, full_name, phone) ON users TO app_auth;
GRANT UPDATE (password_hash, totp_secret_enc, email_verified_at, last_login_at, updated_at)
    ON users TO app_auth;

CREATE POLICY auth_own_memberships ON memberships FOR SELECT TO app_auth
    USING (user_id = nullif(current_setting('app.user_id', true), '')::uuid);
GRANT SELECT (id, user_id, shelter_id, role, status) ON memberships TO app_auth;
```

**The read policy is `USING (true)` and the narrowing is done by the column grant instead** — because
the predicate genuinely cannot be written (we are looking the user up *by* the thing that identifies
them). So `app_auth` can read credentials and cannot read `full_name` or `phone` at all. It can
*write* `full_name` at registration and never read it back: a deliberate asymmetry, and the reason
the PII columns are the ones left out.

`status` is **not** in the UPDATE list: suspending an account is an operator action, not something
the login path can do. `email` is not either: an address change needs re-verification, which is a
Phase 03 flow.

**Rejected: giving `app_auth` a policy on `shelters`.** It never needs one. Shelter rows are read
under tenant scope once the claim exists.

### P2-D4 — B1 on `shelters`: the minimum, and what it forbids

`00015` writes `REVOKE` before `GRANT` (ADR-0009's rule, including `FROM PUBLIC`), then:

```sql
GRANT SELECT ON shelters TO app_tenant;          -- table-level: the policy already scopes it to one row
GRANT INSERT (id, slug, legal_name, display_name, country, state, city,
              mission, vision, about, contact_email, contact_phone, website,
              socials, donation_links) ON shelters TO app_tenant;
GRANT UPDATE (legal_name, display_name, country, state, city,
              mission, vision, about, logo_media_id, cover_media_id,
              contact_email, contact_phone, website, socials, donation_links,
              updated_at) ON shelters TO app_tenant;
-- DELETE is revoked entirely.
```

**What it forbids, and each is a security property from the proposal's table:**

| Forbidden | Why |
|---|---|
| `status` on INSERT and UPDATE | Self-verification bypasses LT-2. The `DEFAULT 'pending_verification'` supplies it, so registration still works `[verified live on PG 17, 2026-09-02: with `GRANT INSERT (id, slug)` only, an INSERT that OMITS `status` succeeds and takes the DEFAULT; one that NAMES it raises **42501**, and so does an UPDATE of an ungranted column. **Gotcha for the test author: the message reads "permission denied for TABLE s", not "for column"** — assert the SQLSTATE, never the wording]` |
| `verified_at`, `verified_by` | Same fact, second and third columns of it |
| `storage_quota_bytes` | Self-raised quota is how a free tier dies |
| `storage_bytes_used` | **Not in the proposal's table, and it is the same hole**: a tenant that can write the counter defeats Phase 04's quota check as thoroughly as writing the quota |
| `slug` on UPDATE | A slug change breaks every public URL already published. Set once at registration |
| `DELETE` | Deleting a shelter row is a destructive operation with no endpoint; archival is a `status` change, and `status` is not writable. Until Phase 06, an operator does it |

**Cost.** Phase 03's shelter-settings endpoint will need `slug` and Phase 06 will need `status`, and
each will arrive as a migration that widens by one column — **visible in a diff**, which is the whole
point of granting the minimum first.

### P2-D5 — B1 on `memberships`: closes self-promotion, and names the hole it does not close

```sql
GRANT SELECT ON memberships TO app_tenant;
GRANT INSERT (id, user_id, shelter_id, role, invited_by, status) ON memberships TO app_tenant;
GRANT UPDATE (status, accepted_at, updated_at) ON memberships TO app_tenant;
-- DELETE is revoked: revocation is `status = 'revoked'`, which keeps the trail.
```

Revoking `UPDATE (role)` closes the stated B1 finding — *"`app_tenant` puede ponerse `role = 'owner'`
en su propia fila"* `[verified: ADR-0009 line 97]`. A promotion now requires a migration.

**The residual, named rather than glossed.** `INSERT` still carries `role`, so a member of shelter A
can insert an `owner` membership for an accomplice account **inside shelter A**. It cannot be closed
by a column grant, because closing it needs the database to know *who is acting*, and under
`app_tenant` there is deliberately no user GUC (P2-D1). Three things bound it: `UNIQUE (user_id,
shelter_id)` blocks a second row for oneself; the tenant policy confines it to one shelter; and
domain RBAC refuses `role IN ('owner','admin')` to anyone who is not `owner`. Phase 03's invitation
endpoint is where the rule becomes expressible.

**So it gets Phase 01's treatment for a known debt: a characterization test.**
`TestMembershipInsert_CanStillMintAnOwner` pins the current behaviour and instructs its future reader
to delete it when Phase 03 narrows the path. A debt with a test on it is a debt somebody finds.

**Rejected: revoking `INSERT` outright.** Registration inserts the founder's `owner` membership
(P2-D6), and moving that insert to `app_auth` would need a policy of `user_id = app.user_id` — under
which **any user could make themselves a member of any existing shelter**. That is a full tenant
takeover, strictly worse than the accomplice path it would close.

**Rejected: `DEFAULT 'volunteer'` on `role` plus revoking `INSERT (role)`.** Tighter, and it breaks
the founder for the same reason.

### P2-D6 — Registration is two transactions, and the compensation is "do nothing"

Shelter registration is open (proposal Decision 6). It spans two roles, so it cannot be one
transaction:

1. `WithAuthUser(newUserID)` — insert the `users` row (`app_auth`). Idempotent on the `citext UNIQUE`
   email.
2. `WithTenant(newShelterID)` — insert `shelters` (with `id = newShelterID`, so `WITH CHECK` passes)
   **and** the founder's `owner` membership, atomically.

If (2) fails, a user exists with no shelter: harmless, and re-running registration resolves it. The
reverse order is impossible — the membership references the user. This is why the ordering is the
design and not an implementation detail.

**Correction to an inherited assumption, and it changes scope.**
`TestApplicantPolicy_IsAsWideAsWritingAnApplication` is documented in the proposal and the exploration
as going red when B1 lands. **It does not.** `[verified: adoption_applications_test.go:338–367 with
seedApplication at :15–36]` — the test's mechanism is an `app_tenant` INSERT into
`adoption_applications` naming an arbitrary `applicant_user_id`. Column grants on `shelters` and
`memberships` do not touch that table, so **the test stays green and is not modified in this phase.**
Closing it would require revoking `INSERT (applicant_user_id)` on `adoption_applications`, which is
the path an adopter uses to apply — Phase 05's flow, which does not exist yet. The marker stays
where Phase 01 put it. Only one pinned test moves in this phase, not two.

### P2-D7 — Assignee active membership: domain check, database backstop is a trigger

**Choice.** RBAC already loads `memberships.role` and `memberships.status` to decide whether the
caller has any role at all, so the domain check is one field away. The database backstop is a
`BEFORE INSERT OR UPDATE` row trigger on `adoption_applications`, guarded by a `WHEN` clause so it
costs nothing on the updates that do not touch the column:

```sql
CREATE FUNCTION assignee_must_be_active_member() RETURNS trigger AS $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM memberships m
                   WHERE m.user_id = NEW.assigned_to_user_id
                     AND m.shelter_id = NEW.shelter_id
                     AND m.status = 'active') THEN
        RAISE EXCEPTION 'assignee % is not an active member of shelter %',
            NEW.assigned_to_user_id, NEW.shelter_id USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;

CREATE TRIGGER adoption_applications_assignee_active
    BEFORE INSERT OR UPDATE OF assigned_to_user_id ON adoption_applications
    FOR EACH ROW WHEN (NEW.assigned_to_user_id IS NOT NULL)
    EXECUTE FUNCTION assignee_must_be_active_member();
```

**Alternatives considered.** (a) A partial `UNIQUE (user_id, shelter_id) WHERE status = 'active'`
referenced by the FK — **not available: a foreign key cannot reference a partial unique index**
`[verified on PG 17, exploration §3.3; not re-litigated]`. (b) Domain check only. (c) A deferrable
`CONSTRAINT TRIGGER`.

**Rationale.** (b) leaves the property with no database backstop, which is what ADR-0002 exists to
prevent and what the security-property table demands. The trigger is not bypassed by a `BYPASSRLS`
role, unlike a policy — ADR-0010's own argument for the append-only triggers, and it applies
unchanged here. The subquery runs as the invoking role, so under `app_tenant` it is filtered by
`memberships`' own tenant policy — which is the correct scope and not a limitation. (c) matters only
if an invitation and an assignment land in one transaction; nothing plans that, and a plain `BEFORE`
trigger fails at the offending statement instead of at `COMMIT`, which is easier to attribute.

**`TestAssignment_DoesNotYetRequireAnActiveMembership` goes red here and is replaced** by its
inverse, in the same commit. This is the one pinned test this phase moves.

### P2-D8 — TOTP: `pquerna/otp`, 160-bit secret, AES-256-GCM with the user id as AAD

**Library:** `github.com/pquerna/otp` — RFC 6238/4226, the de-facto Go implementation, and it
generates the `otpauth://` URI and the QR payload we need. **Rejected:** hand-rolled HMAC (crypto you
write is crypto you maintain forever) and smaller forks (a second-order dependency for a first-order
credential).

**Secret:** 20 bytes from `crypto/rand` (160 bits, RFC 4226's recommendation), base32 for the URI.

**At rest**, in `users.totp_secret_enc bytea` — the column type already implies it `[verified:
00002:50]`. The blob is:

```
version(1) || key_id(1) || nonce(12) || ciphertext || tag(16)
```

AES-256-GCM per §5.4, with **`AAD = user.id` bytes**. That binds the ciphertext to its row: a blob
copied from one user to another fails to open rather than decrypting into a valid secret. Cheap, and
it closes the swap.

**Key management, and where this deliberately stops short of §5.4.** §5.4 describes a data key
wrapped by a KEK. For one column that indirection buys nothing today — both keys would live in the
same environment — so this phase uses **one key from `AUTH_KEK` (base64, 32 bytes) directly**, and
reserves the `key_id` byte for rotation. Phase 07 introduces the wrapped data key when form answers
make the volume justify it, and the format above already carries the bytes needed to migrate without
rewriting the column. Stated here so the deviation from §5.4 is a decision rather than a drift.

**Recovery codes are credentials, so they are hashed** — migration `00014`:

```sql
CREATE TABLE totp_recovery_codes (
    id         uuid        NOT NULL PRIMARY KEY,
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    code_hash  bytea       NOT NULL UNIQUE,   -- sha256 of a 128-bit code
    used_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
```

Ten codes of 128 random bits each, shown once. **SHA-256, not Argon2id** — the same treatment
`refresh_tokens.token_hash` already gets, and for the same reason: a password stretcher exists to
compensate for low entropy, and there is none to compensate for here. `UNIQUE` on `code_hash` for
the reason the token hash has it: two live credentials must never resolve to one row.

Policy `TO app_auth USING/WITH CHECK (user_id = app.user_id)`; grants `SELECT, INSERT, DELETE` and
`UPDATE (used_at)`. A code is marked used, never deleted individually — regenerating the set deletes
all ten and inserts ten more, in one transaction.

**Catalog consequences, and the meta-tests will insist:** `totp_recovery_codes` joins
`rlstest.Schema.NonTenantModel` (it spans shelters, like `users` and `refresh_tokens`);
`refresh_tokens` **leaves `NoPolicy`** — which automatically makes
`TestQueries_ExerciseEveryDeclaredTable` demand a query for it `[verified: query_test.go:104–108,
the exemption is derived and self-expiring]`. Both tables need the "`app_tenant` cannot reach this
at all" assertion that `TestRefreshTokens_RefuseAppTenantEntirely` already models.

### P2-D9 — Cookie, CORS and CSRF: `Origin`/`Sec-Fetch-Site` verification, and a revisit condition the config enforces

Separate origins are decided (proposal Decision 4), so `SameSite=Strict` cannot hold.

**Cookie:** `__Secure-mascotapp_refresh`, `HttpOnly; Secure; SameSite=None; Path=/api/v1/auth`.
The `__Host-` prefix is rejected because it requires `Path=/`, and a refresh cookie sent on every
request is a wider blast radius than the prefix is worth.

**CORS:** an explicit allowlist from `WEB_ORIGINS`, `Access-Control-Allow-Credentials: true`, and
**never `*`** — the wildcard is illegal with credentials anyway, so a misconfiguration fails loudly.

**CSRF: `Origin` / `Sec-Fetch-Site` verification, on every state-changing method and on
`/auth/refresh` specifically.** Rejected: the double-submit token. Rationale:

- The browser sets `Origin` on cross-site requests and **JavaScript cannot forge it**. The allowlist
  it is checked against is *the same list CORS already uses* — one source of truth, nothing to
  store, nothing to rotate, and nothing that a cold start resets.
- Double-submit needs a second, readable cookie, a rotation story, and a client that remembers to
  echo it. It buys the same property with more moving parts and one more thing to get subtly wrong.
- **Fail closed:** a state-changing request with neither `Origin` nor `Sec-Fetch-Site` is refused.
  Honest cost: non-browser clients must send an allowlisted `Origin`, or use the bearer path
  instead. Native clients keep the refresh token in secure storage and send it in the request body —
  no cookie, therefore no CSRF exposure at all.

**The revisit condition is enforced by the config, not by a comment.** `config.Load` computes the
registrable domain of `WEB_ORIGINS` and `API_PUBLIC_ORIGIN` via `golang.org/x/net/publicsuffix` and
**refuses to start with `SameSite=None` when they share one**. The day web and API land on one
hostname, the server says so on boot and points at the ADR. A revisit condition that depends on
someone remembering is a revisit condition that rots — and the user accepted knowingly that this
consolidation may never happen, which is exactly why it must not be quiet.
`[assumed: golang.org/x/net is currently an indirect dependency and becomes direct here. If it is
absent, the fallback is an explicit REGISTRABLE_DOMAINS_DIFFER=true assertion in config with the
same boot refusal — the check stays, the mechanism degrades.]`

### P2-D10 — Rate limiting: in-memory, bounded, per-IP **and** per-account

`internal/httpapi/ratelimit.go`: a sharded `map[key]*bucket` token bucket behind a mutex, read over
`ClientIPFromContext` `[verified: clientip.go:120 exists and is documented as built for exactly
this]`. Registered as middleware on the auth route group only, after the client-IP middleware.

| Endpoint | Per account | Per IP |
|---|---|---|
| `POST /auth/login` | 5 / 15 min | 20 / 15 min |
| `POST /auth/magic-link` | 3 / hour | 10 / hour |

**Two design details that are security properties, not tuning.**

1. **The per-account limiter must key on the submitted email whether or not the account exists.** A
   limiter that only throttles real accounts is an existence oracle over email addresses — the same
   leak proposal Decision 7 closes on the response and the timing, reintroduced through the 429. The
   key is `HMAC-SHA256(process-local random key, normalised email)`, which also keeps addresses out
   of process memory as plaintext and out of any dump (§5.4: logs never contain PII).
2. **The map is bounded.** An unbounded map keyed by attacker-chosen IPs is itself the denial of
   service. Each shard has a cap and evicts the oldest bucket; a janitor drops idle ones. **Stated
   limitation:** an attacker with many addresses can flush per-IP state — and the **per-account**
   limiter is unaffected by that, because its key is the target, not the source. The two limits are
   a pair for this reason, not redundancy.

**The limitation is part of the contract, in the spec text and not only in a comment:** the counter
**resets on every cold start and does not coordinate across instances**. It stops clumsy brute force
and does not stop a patient attacker. Phase 11 owns the version that survives a restart.

### P2-D11 — Access token: HS256, `shelter_id` in the claim, and a mismatch is 403

`golang-jwt/v5`, **HS256** with `JWT_SECRET` (≥32 bytes, refused shorter at boot). Rejected: EdDSA —
asymmetric signing pays for itself when a *second* service verifies without being able to mint, and
there is one service. The reopening condition is exactly that second verifier.

Claims: `iss`, `aud`, `sub` (user id), `exp` (15 min), `iat`, `shelter_id`, `role`, `amr`
(`["pwd","otp"]` — so a route can demand a second factor was actually used, not merely enrolled).

A user in several shelters gets one access token per shelter: `POST /auth/session/shelter` exchanges
a valid session for a token scoped to a chosen shelter, after verifying an **active** membership.

**Tenant middleware:** verify → claims into context → for a tenant-scoped route, require a non-nil
`shelter_id` → `WithTenant`. The router never reads a shelter identifier from a URL or a header
(§5.3, ADR-0008). **And when a request carries one that disagrees with the claim, the answer is an
explicit 403** — not a silent preference for the claim. Silently ignoring the mismatch would let a
client bug, or a probe, look like success. This is success criterion F02, and it is a test.

---

## Data Flow

```
POST /auth/login  (rate limiter: per-IP + per-account, keyed on the email either way)
   │
   ├─ WithAuthLookup ───────────► users  [policy TO app_auth USING(true), credential columns only]
   │        │
   │        ▼  Argon2id verify → TOTP verify (owner/admin) → recovery code fallback
   │
   └─ WithAuthUser(userID) ─────► refresh_tokens INSERT  [policy: user_id = app.user_id]
            │                     users UPDATE last_login_at
            ▼
      Set-Cookie: __Secure-mascotapp_refresh = <b64(user_id)>.<b64(secret)>
                  HttpOnly; Secure; SameSite=None; Path=/api/v1/auth
      body: access token (15 min, HS256)

Any tenant request
   │  Authorization: Bearer <access>
   ▼
verify → claims{sub, shelter_id, role, amr} → RBAC matrix (domain)  ── URL/header shelter_id? → 403
   ▼
db.WithTenant(ctx, tenantPool, claims.ShelterID, fn)
   ▼  set_config('app.shelter_id', $1, true)          [app_tenant: no grant on refresh_tokens]
PostgreSQL RLS  ── unchanged from Phase 01

POST /auth/refresh   (Origin / Sec-Fetch-Site checked BEFORE anything else)
   ▼
parse cookie → user_id prefix → WithAuthUser(user_id) → SELECT by token_hash
   ├─ revoked_at IS NOT NULL → UPDATE ... WHERE family_id = $1   [whole family, one statement]
   └─ valid → rotate: revoke old, insert new with the same family_id
```

---

## File Changes

| File | Action | Description |
|---|---|---|
| `apps/api/internal/db/migrations/00013_auth_role.sql` | Create | `app_auth` role; policies + grants on `refresh_tokens`, `users`, `memberships` (P2-D1…D3) |
| `…/00014_totp_recovery_codes.sql` | Create | `totp_recovery_codes` + RLS + policy + grants (P2-D8) |
| `…/00015_column_grants.sql` | Create | B1: `REVOKE` then column `GRANT` on `shelters`, `memberships` (P2-D4, P2-D5) |
| `…/00016_assignee_active_membership.sql` | Create | Trigger function + trigger (P2-D7) |
| `apps/api/internal/db/auth.go` | Create | `WithAuthUser`, `WithAuthLookup`, `ErrNoAuthUser`, `NewAuthPool` |
| `apps/api/internal/db/query/auth.sql` | Create | sqlc inputs for `refresh_tokens`, `totp_recovery_codes`, auth-scoped `users`/`memberships` |
| `apps/api/internal/db/rlstest/catalog.go` | Modify | `refresh_tokens` leaves `NoPolicy`; `totp_recovery_codes` joins `NonTenantModel` |
| `apps/api/internal/db/dbtest/container.go`, `roles.go` | Modify | `AuthPool`, `app_auth` password bootstrap, role guard over the third pool |
| `apps/api/internal/auth/` | Create | `password.go` (Argon2id), `totp.go`, `envelope.go` (AES-256-GCM), `token.go` (JWT), `session.go` (rotation + reuse detection), `recovery.go` |
| `apps/api/internal/authz/` | Create | `matrix.go` — the permission matrix by `membership.role`, pure and table-driven |
| `apps/api/internal/httpapi/auth_handlers.go` | Create | register, login, magic-link, verify, refresh, logout, TOTP enrol/verify, recovery |
| `apps/api/internal/httpapi/middleware_auth.go` | Create | bearer verification, claims context, tenant middleware, the 403-on-mismatch rule |
| `apps/api/internal/httpapi/cors.go`, `csrf.go` | Create | allowlist with credentials; `Origin`/`Sec-Fetch-Site` verification (P2-D9) |
| `apps/api/internal/httpapi/ratelimit.go` | Create | bounded sharded token bucket (P2-D10) |
| `apps/api/internal/httpapi/router.go` | Modify | `/api/v1/auth` group with limiter + CSRF; tenant group with the auth middleware |
| `apps/api/internal/email/` | Create | `Sender` port + `LogSender` stub (proposal Decision 2) |
| `apps/api/internal/config/config.go` | Modify | auth DSN, `JWT_SECRET`, `AUTH_KEK`, `WEB_ORIGINS`, `API_PUBLIC_ORIGIN`, same-site boot refusal |
| `api/openapi.yaml` | Modify | the auth surface (`oapi-codegen` regenerates `internal/api/openapi.gen.go`) |
| `apps/api/go.mod` | Modify | `golang-jwt/v5`, `pquerna/otp`, `golang.org/x/crypto` (direct), `golang.org/x/net` |
| `.env.example` | Modify | **Blocked pending user confirmation — see Open Questions** |

---

## Interfaces / Contracts

```go
// The email port. One method, so the stub is honest and Resend is a later adapter.
type Sender interface {
    Send(ctx context.Context, to string, msg Message) error
}

// The permission matrix is pure data and a pure function: no database, no
// context, no clock. Every role x permission cell is a test case.
type Permission string
type Role       string

func Allows(role Role, p Permission) bool

// Claims is what the middleware puts in the context, and the ONLY source of
// shelter_id anywhere above internal/db.
type Claims struct {
    UserID    uuid.UUID
    ShelterID uuid.UUID // uuid.Nil for an adopter session
    Role      Role
    AMR       []string
}
```

---

## Testing Strategy

Strict TDD, RED first. Runner is **`make test-api-container`** — a bare `go test` does not run on
this host `[verified: Makefile:119–125; Windows Smart App Control blocks freshly linked test
binaries]`. `openspec/config.yaml` still records the bare form; tasks carry the container runner.

| Layer | What to test | Approach |
|---|---|---|
| Unit | `WithAuthUser` refuses `uuid.Nil`, sets `app.user_id` with `is_local = true`, rolls back on error and on panic | recording `pgx.Tx` stub, no database — the same shape that let T-01-003 mutation-test `WithTenant` |
| Unit | Argon2id params; envelope encrypt/decrypt round-trip; **a blob re-tagged with another user id fails to open** (the AAD binding) | pure Go, `t.Parallel()` |
| Unit | Every role × permission cell of the matrix | table-driven, exhaustive by construction |
| Unit | Rate limiter: bucket refill, shard eviction, and **the same 429 shape and timing for a known and an unknown address** | fake clock |
| Unit | CSRF: absent `Origin` and absent `Sec-Fetch-Site` on a state-changing method is refused | `httptest` |
| Unit | Config refuses `SameSite=None` when web and API share a registrable domain | table of origin pairs |
| Integration | `refresh_tokens` reachable through `app_auth`, **still refused to `app_tenant`** (read and write) | third pool, mirrors `TestRefreshTokens_RefuseAppTenantEntirely` |
| Integration | A user scoped to A cannot see B's sessions or recovery codes under `app_auth` | the A/B shape, keyed on `user_id` |
| Integration | `app_tenant` writing `shelters.status`, `storage_quota_bytes`, `storage_bytes_used`, or `memberships.role` fails with **`42501`**; and the anti-vacuity case — a permitted column still writes | per column, enumerated |
| Integration | Assigning to an `invited` or `revoked` member raises `23514`; assigning to an active one succeeds | replaces `TestAssignment_DoesNotYetRequireAnActiveMembership` |
| Integration | Refresh reuse revokes the whole `family_id` | end-to-end through the handler |
| Meta | `TestPolicies_DoNotCrossGUCs` — no `app_tenant` policy mentions `app.user_id` and vice versa | `pg_policy` + `pg_get_expr` |
| Meta | Catalog set updated: `totp_recovery_codes` classified, `refresh_tokens` out of `NoPolicy`, and `TestQueries_ExerciseEveryDeclaredTable` now demands its query | existing meta-tests, which fail loudly on their own |
| E2E | Forged `shelter_id` in a URL or header → 403; `owner`/`admin` cannot complete login without TOTP | `httptest` + container |

Coverage gate ≥ 75%, per the proposal's success criteria.

---

## Threat Matrix

| Boundary | Applicability | Reason |
|---|---|---|
| Documentation-like paths | **N/A** | This change classifies no file as executable and reads no file as configuration at runtime |
| Git repository selection | **N/A** | No VCS invocation anywhere in the change |
| Commit state | **N/A** | No index or worktree semantics |
| Push state | **N/A** | No ref resolution |
| PR commands | **N/A** | No PR automation, no composed shell commands, no subprocess |

The change adds **HTTP** routing, not shell, subprocess, VCS or executable-file classification. Its
adversarial surface — forged `shelter_id`, CSRF under `SameSite=None`, refresh reuse, brute force,
privilege escalation within a tenant — is owned by the proposal's security-property table and by the
integration and E2E rows above, each of which is a RED test before its production change.

---

## Migration / Rollout

Per `proposal.md`'s rollback plan. Two refinements it asks design to settle:

- **The `app_auth` ordering hazard dissolves**: `00013`'s `Down` revokes the grants and drops the
  policies but **does not drop the role** — the same asymmetry `00001` applies to `app_tenant` and
  `app_public`, and for the same reason (a role may be shared with another database in the cluster)
  `[verified: 00001 lines 79–84]`.
- **`00015`'s `Down` restores table-level grants**, which re-opens B1. That is what a rollback of
  this change means, and it is stated rather than discovered.
- **Rolling back invalidates every issued refresh token.** All sessions end. Accepted in the
  proposal; repeated here because it is the only irreversible cost in the set.

Delivery follows the proposal's five slices, which map onto the migrations: (a) `00013` + the auth
door, (b) `00015`/`00016` + the two test updates, (c) the auth HTTP surface + `00014`, (d) the RBAC
matrix, (e) limiter + email stub.

---

## Open Questions

- [ ] **`.env.example` is denied to agents, so the presence of `JWT_SECRET` and `RESEND_API_KEY` is
      unverified, not confirmed absent.** The keys this design needs, for the user to confirm or add:
      `DATABASE_URL_AUTH`, `AUTH_KEK` (base64, 32 bytes), `JWT_SECRET` (≥32 bytes), `WEB_ORIGINS`,
      `API_PUBLIC_ORIGIN`, and the `app_auth` bootstrap password variable. `RESEND_API_KEY` is **not**
      needed this phase — the stub has no external dependency. **Blocking for apply, not for tasks.**
- [ ] `config.go` today loads a single `DATABASE_URL` `[verified: config.go:54]`, while the Phase 01
      design named `DATABASE_URL_TENANT` / `DATABASE_URL_PUBLIC`. Whether that split already exists in
      the environment, or lands here, needs the same confirmation.
- [ ] `golang.org/x/net/publicsuffix` becoming a direct dependency (P2-D9). Fallback stated in the
      decision; the boot refusal is not optional either way.
- [ ] Recovery-code display is one-shot. Whether the UI forces an acknowledgement before continuing is
      a Phase 10 question; the API returns them exactly once and never again, which is the part that
      belongs here.
