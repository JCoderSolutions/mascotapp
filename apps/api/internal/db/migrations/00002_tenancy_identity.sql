-- Tenancy and identity: shelters, users, memberships, refresh_tokens.
--
-- This is the first migration that creates a table, so it is the first one that
-- has to get the row-level-security template right. Every later migration
-- replays it verbatim; T-01-016 reviews it once, here, rather than four times.
--
-- Primary keys are `uuid NOT NULL` with NO database default (D3). Identifiers
-- are time-ordered UUID v7 generated in Go before the insert, because
-- pg_uuidv7 is not available on Neon. A default would make that silently
-- optional and let a v4 leak in the day somebody forgets.
--
-- Table order below is dictated by foreign keys: `users` first, because
-- `shelters.verified_by` points at it.

-- +goose Up

-- +goose StatementBegin

-- users spans shelters and therefore carries NO shelter_id (D6). It is visible
-- to a tenant only through memberships, via the policy at the bottom of this
-- file.
--
-- Nullable columns are nullable because §4.1 does not require them: an adopter
-- who signed in with a magic link has no password_hash and may not have given a
-- name yet. NOT NULL is spent where the plan asks for it and where a missing
-- value would be a correctness bug, not where it would only force a caller to
-- invent a placeholder.
CREATE TABLE users (
    id                uuid        NOT NULL PRIMARY KEY,

    -- citext, exactly as §4.1 writes it (D7, reversed 2026-08-29). The
    -- case-insensitivity is a property of the COLUMN TYPE, so a lookup written
    -- as `WHERE email = $1` matches regardless of casing on either side. A
    -- lower(email) unique index would enforce the same uniqueness while
    -- leaving every lookup to caller discipline -- the exact failure mode
    -- ADR-0002 exists to remove.
    email             citext      NOT NULL UNIQUE,

    -- NULL means "magic-link only". Passwords that do not exist cannot leak.
    password_hash     text,
    full_name         text,
    phone             text,

    status            text        NOT NULL DEFAULT 'active'
                                  CHECK (status IN ('active', 'suspended', 'deleted')),
    email_verified_at timestamptz,

    -- Encrypted at the application layer before it reaches this column, so the
    -- type is bytea rather than text.
    totp_secret_enc   bytea,
    last_login_at     timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

-- A shelter's rows ARE the tenants, so `shelters` is scoped by its own `id`
-- rather than by a shelter_id column. That is the one substitution the policy
-- template allows.
CREATE TABLE shelters (
    id                  uuid        NOT NULL PRIMARY KEY,
    slug                text        NOT NULL UNIQUE,
    legal_name          text,
    display_name        text        NOT NULL,
    country             text,
    state               text,
    city                text,

    -- LT-2: a shelter cannot publish until a human verifies it, so the default
    -- has to be the unverified state. A default of 'verified' would make
    -- forgetting to set it a security failure instead of a visible one.
    status              text        NOT NULL DEFAULT 'pending_verification'
                                    CHECK (status IN ('pending_verification', 'verified',
                                                      'suspended', 'archived')),
    verified_at         timestamptz,
    verified_by         uuid        REFERENCES users (id),

    mission             text,
    vision              text,
    about               text,

    -- No foreign key yet: `media` is migration 00003 and a constraint cannot
    -- reference a table that does not exist. The key is added there, and
    -- TestShelters_MediaColumnsGetTheirForeignKeyWhenMediaLands fails the
    -- moment `media` lands without it -- so this deferral cannot be forgotten
    -- the way a TODO comment can.
    logo_media_id       uuid,
    cover_media_id      uuid,

    contact_email       citext,
    contact_phone       text,
    website             text,
    socials             jsonb       NOT NULL DEFAULT '{}'::jsonb,

    -- [{provider,label,url,verified_at}]. The platform never processes a
    -- payment (LT-3); it only links to a payment page the shelter owns and a
    -- human verified during onboarding.
    donation_links      jsonb       NOT NULL DEFAULT '[]'::jsonb,

    storage_bytes_used  bigint      NOT NULL DEFAULT 0,

    -- NOT NULL with a default rather than nullable: a NULL quota reads as "no
    -- quota", and a shelter with unbounded storage is how a free tier dies. The
    -- number itself is a placeholder that Phase 04 owns.
    storage_quota_bytes bigint      NOT NULL DEFAULT 1073741824,

    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

-- A membership joins ONE user to ONE shelter, so by D6's own test it
-- denormalises shelter_id and takes the standard direct policy. It is not an
-- EXISTS carve-out and it is not a D5 child: its shelter_id is its own, not
-- copied down from a parent, so a single-column foreign key is correct here.
CREATE TABLE memberships (
    id          uuid        NOT NULL PRIMARY KEY,
    user_id     uuid        NOT NULL REFERENCES users (id),
    shelter_id  uuid        NOT NULL REFERENCES shelters (id),
    role        text        NOT NULL
                            CHECK (role IN ('owner', 'admin', 'staff', 'volunteer')),
    status      text        NOT NULL DEFAULT 'invited'
                            CHECK (status IN ('invited', 'active', 'revoked')),
    invited_by  uuid        REFERENCES users (id),
    accepted_at timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),

    UNIQUE (user_id, shelter_id)
);

-- Identity-domain, and DEFAULT-DENY for this phase: row-level security is
-- enabled with no policy and no grant at all, which refuses app_tenant
-- outright. That is stricter than any policy this phase could write, because
-- the right scope for a refresh token is the USER, and no app.user_id GUC
-- exists yet. Phase 02 chooses between a dedicated app_auth role and that GUC.
CREATE TABLE refresh_tokens (
    id          uuid        NOT NULL PRIMARY KEY,
    user_id     uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,

    -- The token itself is never stored. This is the lookup key, so a duplicate
    -- would mean two live tokens resolving to one row.
    token_hash  bytea       NOT NULL UNIQUE,

    -- Reuse of any token in a family revokes the whole family (§5.2), so the
    -- family is a first-class column rather than something walked through
    -- replaced_by.
    family_id   uuid        NOT NULL,
    expires_at  timestamptz NOT NULL,
    revoked_at  timestamptz,
    replaced_by uuid        REFERENCES refresh_tokens (id),
    user_agent  text,

    -- Hashed with a salt, never the address itself (§5.4).
    ip_hash     bytea,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Row-level security.
--
-- ENABLE makes the policies apply. FORCE makes them apply to the table OWNER
-- too, which is what stops a migration or an admin connection from silently
-- bypassing them. Neither applies to a superuser or a role holding BYPASSRLS,
-- which is why 00001 creates the application roles in SQL with NOBYPASSRLS
-- rather than through the Neon Console.
--
-- nullif(current_setting(..., true), '') -- BOTH halves are required, and the
-- nullif is not defensive programming, it is a bug fix. `true` is missing_ok, so
-- a GUC that was NEVER set yields NULL. But a GUC that was set inside a
-- transaction and reverted at COMMIT or ROLLBACK does not go back to unset: it
-- goes back to the EMPTY STRING. So on a pooled connection that has already
-- served one scoped request -- which is every connection after its first use --
-- the bare expression is `shelter_id = ''::uuid` and raises 22P02 instead of
-- returning zero rows.
--
-- Found by T-01-015 on 2026-08-30, against a real pool. The failure is loud
-- rather than leaky, so nothing was ever exposed; what was broken is that the
-- fail-closed contract held or did not hold depending on whether that particular
-- connection had been used before. `nullif` makes the empty string and the unset
-- case identical, so an unscoped query returns zero rows in both.
--
-- The original note follows, and it is still the reason the `true` is there:
-- current_setting(..., true) -- the `true` is missing_ok. An unset GUC yields
-- NULL, the comparison yields NULL, and the query returns zero rows. Fail-closed
-- by construction: a query issued outside WithTenant sees nothing, never the
-- wrong rows.
--
-- WITH CHECK is written out even though PostgreSQL infers it from USING for a
-- FOR ALL policy, because that inference disappears the moment a policy is split
-- per command -- as it is for the append-only tables later in this phase.
-- ---------------------------------------------------------------------------

ALTER TABLE shelters ENABLE ROW LEVEL SECURITY;
ALTER TABLE shelters FORCE  ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON shelters FOR ALL TO app_tenant
    USING      (id = nullif(current_setting('app.shelter_id', true), '')::uuid)
    WITH CHECK (id = nullif(current_setting('app.shelter_id', true), '')::uuid);

ALTER TABLE memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE memberships FORCE  ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON memberships FOR ALL TO app_tenant
    USING      (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid)
    WITH CHECK (shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid);

ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE  ROW LEVEL SECURITY;

-- D6's single sanctioned carve-out. A user row belongs to many shelters, so it
-- cannot carry a shelter_id, and its visibility is derived through memberships
-- instead. Migration 00009 adds a SECOND permissive policy for the applicant
-- path once adoption_applications exists; permissive policies OR together, and
-- both are scoped TO app_tenant.
--
-- FOR SELECT only. No tenant may create, edit or delete a user in this phase --
-- the write path belongs to Phase 02's registration flow -- and the grant below
-- says the same thing a second time, deliberately.
--
-- The `AND m.shelter_id = ...` below is REDUNDANT, and it is kept on purpose.
-- Proven by mutation on 2026-08-30 (T-01-014): deleting that line alone changes
-- nothing observable, because a policy expression is evaluated as the querying
-- role, so this subquery is ITSELF filtered by memberships' own policy -- the
-- same mechanism behind PostgreSQL's familiar "infinite recursion detected in
-- policy" error. Deleting it AND adding a permissive SELECT policy to
-- memberships does leak, which is how the mechanism was confirmed rather than
-- assumed.
--
-- So this table's visibility is the CONJUNCTION of two policies, and the second
-- one lives on another table. Written out here because a reader who notices the
-- redundancy and removes it leaves users' isolation resting entirely on a rule
-- that is nowhere near this file.
--
-- `AND m.status = 'active'` is load-bearing, not decoration. §4.1's own
-- wording is "user U has an ACTIVE membership in shelter A", and without this
-- predicate the policy grants more than that: app_tenant holds INSERT and
-- UPDATE on memberships (grants below), so a tenant could INSERT an `invited`
-- membership row for an arbitrary user id and thereby mint visibility of that
-- user's PII row for itself -- no acceptance required. Worse, revoking that
-- membership (UPDATE ... SET status = 'revoked') would not take the
-- visibility back, because 'invited' and 'revoked' are just as visible as
-- 'active' once the status column is left unfiltered. Confirmed live against
-- PostgreSQL 17 on 2026-08-30: an `invited` row and, separately, that same row
-- flipped to `revoked` both kept the user's row selectable; only filtering on
-- `status = 'active'` closes it.
CREATE POLICY member_visible_users ON users FOR SELECT TO app_tenant
    USING (EXISTS (SELECT 1
                   FROM memberships m
                   WHERE m.user_id = users.id
                     AND m.shelter_id = nullif(current_setting('app.shelter_id', true), '')::uuid
                     AND m.status = 'active'));

ALTER TABLE refresh_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE refresh_tokens FORCE  ROW LEVEL SECURITY;

-- Deliberately no policy on refresh_tokens. See the table comment above.

-- ---------------------------------------------------------------------------
-- Grants, per table and never ON ALL TABLES (D9). A table stays unreachable
-- until a migration grants it on purpose, which is what lets the catalog
-- meta-test fail loudly on an omission instead of shrugging.
-- ---------------------------------------------------------------------------

GRANT SELECT, INSERT, UPDATE, DELETE ON shelters    TO app_tenant;
GRANT SELECT, INSERT, UPDATE, DELETE ON memberships TO app_tenant;

-- SELECT only: see member_visible_users above.
GRANT SELECT                        ON users        TO app_tenant;

-- refresh_tokens gets no grant at all.

-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin

-- Policies and grants are dropped with their tables, so they are not listed
-- separately. The extension and the roles are NOT dropped here for the same
-- reasons 00001 gives: DROP EXTENSION citext would cascade to users.email, and
-- a role may be shared with another database in the cluster.
--
-- member_visible_users lives ON users but READS memberships, and PostgreSQL
-- records that as a dependency: dropping memberships while the policy exists
-- fails with 2BP01. It has to come down first, and by name.
--
-- Not with DROP TABLE ... CASCADE. CASCADE would take this policy and anything
-- else that ever comes to depend on these tables, silently — a rollback whose
-- blast radius is whatever the schema happens to contain on the day is not a
-- rollback anybody can review. The stepwise round-trip test found this.
DROP POLICY member_visible_users ON users;

-- Reverse creation order, because the foreign keys run the other way.
DROP TABLE refresh_tokens;
DROP TABLE memberships;
DROP TABLE shelters;
DROP TABLE users;

-- +goose StatementEnd
