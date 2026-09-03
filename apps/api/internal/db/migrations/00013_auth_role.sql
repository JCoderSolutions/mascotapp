-- The auth door: a third role, its policies, and its grants (P2-D1..P2-D3).
--
-- Phase 01 built two doors, both keyed on app.shelter_id. Identity work does
-- not fit either. A session row belongs to a user, not to a shelter, and the
-- credential lookup that precedes login has no shelter to be scoped by -- there
-- is no claim yet. Widening app_tenant to cover it would put a second GUC on a
-- role whose entire safety argument is that one GUC decides everything it can
-- see.
--
-- So app_auth is a third door, not a second key on the existing one. It reads
-- app.user_id and never app.shelter_id, and app_tenant never reads app.user_id.
-- TestPolicies_DoNotCrossGUCs (T-02-004) is what keeps that true after this
-- migration stops being the newest one.
--
-- Nothing here sets a password, for the reason 00001 states: ALTER ROLE ...
-- PASSWORD is a utility statement and takes no bind parameters, so a password
-- in a migration is a secret in a committed file, in the embedded binary, and
-- in `goose status` output. SetRolePassword in bootstrap.go owns that (D8).

-- +goose Up

-- +goose StatementBegin
-- CREATE ROLE is cluster-wide, so the guard on pg_roles is what makes replaying
-- this migration against a reused cluster safe -- same reason as 00001.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_auth') THEN
        CREATE ROLE app_auth
            NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS NOINHERIT LOGIN;
    END IF;
END
$$;
-- +goose StatementEnd

-- Re-asserted rather than assumed. If the role already existed in a reused
-- cluster -- carrying SUPERUSER or BYPASSRLS from somewhere else -- every policy
-- below would be silently inert, and the harness role guard would be the only
-- thing between that and a green suite that proves nothing.
ALTER ROLE app_auth NOSUPERUSER NOBYPASSRLS;

GRANT USAGE ON SCHEMA public TO app_auth;

-- refresh_tokens (P2-D2). The table has had RLS enabled and FORCEd since 00002
-- with no policy at all, which is what has been denying every non-owner role
-- outright. This is the first policy it gets, and app_tenant/app_public still
-- receive no grant on it: the rotation path is reachable through this door and
-- through nothing else.
--
-- WITH CHECK repeats USING deliberately. USING filters what a statement can
-- see; WITH CHECK is what refuses a forged write. Without it, an INSERT naming
-- another user's id would be accepted.
CREATE POLICY auth_own_sessions ON refresh_tokens FOR ALL TO app_auth
    USING      (user_id = nullif(current_setting('app.user_id', true), '')::uuid)
    WITH CHECK (user_id = nullif(current_setting('app.user_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON refresh_tokens TO app_auth;

-- users (P2-D3). Three policies rather than one, because the three paths have
-- genuinely different shapes and a single policy with an OR inside it is one
-- edit away from widening all three at once -- the same argument D6 makes for
-- keeping applicant_visible_users and member_visible_users apart.
--
-- The read is USING (true) and that is not an oversight: the credential lookup
-- runs BEFORE anyone is authenticated, so there is no app.user_id to scope it
-- by. The narrowing is done by column grant instead of by row predicate -- this
-- role can read the columns login needs and cannot read full_name or phone at
-- all. A row predicate here could only be written by inventing a scope that
-- does not exist yet.
CREATE POLICY auth_lookup_users ON users FOR SELECT TO app_auth USING (true);

CREATE POLICY auth_own_user ON users FOR UPDATE TO app_auth
    USING      (id = nullif(current_setting('app.user_id', true), '')::uuid)
    WITH CHECK (id = nullif(current_setting('app.user_id', true), '')::uuid);

CREATE POLICY auth_register_user ON users FOR INSERT TO app_auth WITH CHECK (true);

-- Column grants, not table grants. A column privilege is checked only for the
-- columns a statement NAMES, so registration can insert a row and let status
-- take its DEFAULT without ever holding a privilege on status.
--
-- Note for whoever writes the negative assertions: a column-privilege denial
-- reports `permission denied for TABLE users` -- it says TABLE, not column.
-- Assert the SQLSTATE 42501, never the message text.
GRANT SELECT (id, email, password_hash, status, totp_secret_enc, email_verified_at, created_at)
    ON users TO app_auth;
GRANT INSERT (id, email, password_hash, full_name, phone) ON users TO app_auth;
GRANT UPDATE (password_hash, totp_secret_enc, email_verified_at, last_login_at, updated_at)
    ON users TO app_auth;

-- memberships (P2-D3). Read-only and scoped to the caller's own rows: the auth
-- door needs to know which shelters a user belongs to in order to mint a claim,
-- and needs nothing else. It never writes here -- the founder's membership is
-- inserted under app_tenant, inside registration's second transaction (P2-D6).
CREATE POLICY auth_own_memberships ON memberships FOR SELECT TO app_auth
    USING (user_id = nullif(current_setting('app.user_id', true), '')::uuid);

GRANT SELECT (id, user_id, shelter_id, role, status) ON memberships TO app_auth;

-- +goose Down

-- Grants and policies come off; the ROLE stays. Same asymmetry as 00001, and
-- for the same reason: CREATE ROLE is cluster-wide, so dropping it here would
-- reach outside this database and break anything else connected as it. The
-- rollback round-trip test asserts app_auth survives `goose down`.
REVOKE SELECT (id, user_id, shelter_id, role, status) ON memberships FROM app_auth;
DROP POLICY IF EXISTS auth_own_memberships ON memberships;

REVOKE UPDATE (password_hash, totp_secret_enc, email_verified_at, last_login_at, updated_at)
    ON users FROM app_auth;
REVOKE INSERT (id, email, password_hash, full_name, phone) ON users FROM app_auth;
REVOKE SELECT (id, email, password_hash, status, totp_secret_enc, email_verified_at, created_at)
    ON users FROM app_auth;
DROP POLICY IF EXISTS auth_register_user ON users;
DROP POLICY IF EXISTS auth_own_user ON users;
DROP POLICY IF EXISTS auth_lookup_users ON users;

REVOKE SELECT, INSERT, UPDATE, DELETE ON refresh_tokens FROM app_auth;
DROP POLICY IF EXISTS auth_own_sessions ON refresh_tokens;

REVOKE USAGE ON SCHEMA public FROM app_auth;
