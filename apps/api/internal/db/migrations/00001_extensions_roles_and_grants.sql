-- Extensions, application roles, and the schema-level grants they start from.
--
-- This runs before any table exists, and deliberately so: a policy names the
-- role it applies TO, so every role has to be in place before the first table
-- enables row-level security.
--
-- Nothing here sets a password. ALTER ROLE ... PASSWORD is a utility statement
-- and cannot take bind parameters -- the same trap as SET LOCAL -- so putting it
-- in a migration would force the secret into a file that is committed, embedded
-- in the binary, and printed by `goose status`. Passwords are set out of band by
-- SetRolePassword in bootstrap.go (design decision D8).

-- +goose Up

-- +goose StatementBegin

-- citext is the only extension this schema uses. Verified 2026-08-29 to be
-- available in both target environments: Neon documents it, and the compose
-- image postgres:17-alpine ships citext 1.6 out of the box.
--
-- pg_uuidv7 is deliberately NOT used (D3): it is not available on Neon, so
-- UUIDv7 values are generated in Go instead.
CREATE EXTENSION IF NOT EXISTS citext;

-- Roles are created here, in SQL, and never through the Neon Console, CLI or
-- API. This is not a preference. neon_superuser carries the BYPASSRLS
-- attribute and is granted automatically to every role created through those
-- interfaces, including the default project role. A hand-provisioned app_tenant
-- would therefore make every policy in this schema a silent no-op in
-- production, while the local A/B isolation suite kept passing -- the compose
-- Postgres has no neon_superuser to inherit it from.
--
-- CREATE ROLE is cluster-wide, so the guard on pg_roles is what makes replaying
-- this migration against a second database in the same cluster safe.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_tenant') THEN
        CREATE ROLE app_tenant
            NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS NOINHERIT LOGIN;
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_public') THEN
        CREATE ROLE app_public
            NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS NOINHERIT LOGIN;
    END IF;
END
$$;

-- Re-applied unconditionally rather than inside the guard above: the roles may
-- already exist from another database in this cluster, but the grants below are
-- per-database and would then be missing.
--
-- NOSUPERUSER and NOBYPASSRLS are re-asserted for the same reason. If a role
-- was created by hand with either attribute, this strips it, and the suite's
-- pg_roles guard test would otherwise be the only thing standing between that
-- and production.
ALTER ROLE app_tenant NOSUPERUSER NOBYPASSRLS;
ALTER ROLE app_public NOSUPERUSER NOBYPASSRLS;

-- PUBLIC grants CREATE on the public schema by default in PostgreSQL versions
-- before 15 and is worth revoking explicitly regardless: neither application
-- role has any business creating objects.
REVOKE ALL ON SCHEMA public FROM PUBLIC;

GRANT USAGE ON SCHEMA public TO app_tenant;
GRANT USAGE ON SCHEMA public TO app_public;

-- No ON ALL TABLES, and no ALTER DEFAULT PRIVILEGES anywhere in this schema
-- (D9). A new table is unreachable by either role until a migration grants it
-- deliberately, which is what lets the catalog meta-test fail loudly on a table
-- somebody forgot to classify.

-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin

-- Grants are revoked. Roles are NOT dropped, and the extension is NOT dropped.
--
-- DROP ROLE would fail or cascade if the role owns objects or is in use by
-- another database in this cluster (D1b). DROP EXTENSION citext would cascade
-- to users.email and destroy the column that `goose down-to` is supposed to
-- leave recoverable. Both asymmetries are deliberate.
REVOKE USAGE ON SCHEMA public FROM app_tenant;
REVOKE USAGE ON SCHEMA public FROM app_public;

-- +goose StatementEnd
