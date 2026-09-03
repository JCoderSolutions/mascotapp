-- B1: a tenant stops being able to write the columns that decide its own
-- privileges (P2-D4 for `shelters`, P2-D5 for `memberships`).
--
-- Phase 01 granted `SELECT, INSERT, UPDATE, DELETE` on both tables table-wide.
-- RLS never had anything to say about it: a policy decides WHICH ROWS a role
-- reaches, and every write this migration closes lands on a row the tenant
-- legitimately owns. `app_tenant` could set its own `status = 'verified'`,
-- raise its own `storage_quota_bytes`, zero the `storage_bytes_used` counter
-- that quota is checked against, rewrite its published `slug`, and promote
-- itself to `role = 'owner'`. Only a COLUMN privilege can refuse that, which is
-- what this migration installs.
--
-- The semantic it rests on, verified live on PG 17 (2026-09-02) and pinned in
-- `rlstest/column_privilege_test.go` so a future grant change cannot break
-- registration in silence: a column privilege is consulted only for the columns
-- a statement NAMES. An INSERT that omits `status` succeeds and takes the
-- DEFAULT; one that names it raises 42501. That is why registration still works
-- while holding no privilege on `status` at all.
--
-- Note for whoever writes assertions against this: the refusal reads
-- `permission denied for TABLE shelters` -- it says TABLE, not column. Assert
-- the SQLSTATE 42501, never the message text.
--
-- REVOKE comes before GRANT, `FROM PUBLIC` included, per ADR-0009. A privilege
-- nobody revoked is a privilege that survives, and `REVOKE` on something nobody
-- granted is a harmless no-op -- which is the cheap half of a rule whose
-- expensive half is a grant that quietly outlives the migration meant to
-- replace it.
--
-- What this deliberately does NOT close, named rather than glossed: `INSERT`
-- still carries `role`, so a member of shelter A can mint an `owner` membership
-- for an accomplice INSIDE shelter A. A column grant cannot reach it -- closing
-- it needs the database to know WHO is acting, and under `app_tenant` there is
-- no user GUC by design (P2-D1). Revoking INSERT outright was rejected: the
-- founder's own membership is inserted on this path, and moving it to `app_auth`
-- would need `user_id = app.user_id`, under which any user could join any
-- existing shelter -- a full tenant takeover, strictly worse than the hole it
-- would close. `TestMembershipInsert_CanStillMintAnOwner` pins the residual and
-- tells its reader to delete it when Phase 03's invitation endpoint lands.

-- +goose Up

-- +goose StatementBegin

-- ---------------------------------------------------------------------------
-- shelters (P2-D4)
-- ---------------------------------------------------------------------------

REVOKE ALL ON shelters FROM PUBLIC;
REVOKE ALL ON shelters FROM app_tenant;

-- SELECT stays table-wide: the tenant policy already narrows it to one row, and
-- a shelter reading its own record in full is the product working.
GRANT SELECT ON shelters TO app_tenant;

-- INSERT carries `slug` -- a shelter is named once, at registration. It does not
-- carry `status`, `verified_at`, `verified_by`, `storage_quota_bytes` or
-- `storage_bytes_used`: each of those has a DEFAULT, so registration inserts a
-- row without ever holding a privilege on them.
GRANT INSERT (id, slug, legal_name, display_name, country, state, city,
              mission, vision, about, contact_email, contact_phone, website,
              socials, donation_links) ON shelters TO app_tenant;

-- UPDATE drops `slug`: changing it breaks every public URL already published.
-- Phase 03's shelter-settings endpoint will need it back and Phase 06 will need
-- `status`; each arrives as a migration that widens by one column, VISIBLE IN A
-- DIFF. That visibility is the entire reason for granting the minimum first.
GRANT UPDATE (legal_name, display_name, country, state, city,
              mission, vision, about, logo_media_id, cover_media_id,
              contact_email, contact_phone, website, socials, donation_links,
              updated_at) ON shelters TO app_tenant;

-- DELETE is not re-granted. Removing a shelter row is destructive with no
-- endpoint behind it; archival is a `status` change, and `status` is not
-- writable either. Until Phase 06 an operator does it.

-- ---------------------------------------------------------------------------
-- memberships (P2-D5)
-- ---------------------------------------------------------------------------

REVOKE ALL ON memberships FROM PUBLIC;
REVOKE ALL ON memberships FROM app_tenant;

GRANT SELECT ON memberships TO app_tenant;

GRANT INSERT (id, user_id, shelter_id, role, invited_by, status)
    ON memberships TO app_tenant;

-- `role` is absent, and that is the B1 finding closed: ADR-0009 recorded that
-- `app_tenant` could set `role = 'owner'` on its own row. A promotion now
-- requires a migration.
GRANT UPDATE (status, accepted_at, updated_at) ON memberships TO app_tenant;

-- DELETE is not re-granted: revocation is `status = 'revoked'`, which keeps the
-- trail. A DELETE erases the evidence that the membership ever existed.

-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin

-- Back to Phase 01's table-wide grants, exactly as `00002` wrote them. The
-- REVOKE first is not ceremony: without it the column grants installed above
-- survive alongside the table grants, and the schema after a down is not the
-- schema before the up. `TestMigrations_EveryStepDownLeavesAConsistentSchema`
-- walks every intermediate version and is what makes that assertion rather than
-- a hope.
REVOKE ALL ON memberships FROM app_tenant;
REVOKE ALL ON shelters    FROM app_tenant;

GRANT SELECT, INSERT, UPDATE, DELETE ON shelters    TO app_tenant;
GRANT SELECT, INSERT, UPDATE, DELETE ON memberships TO app_tenant;

-- +goose StatementEnd
