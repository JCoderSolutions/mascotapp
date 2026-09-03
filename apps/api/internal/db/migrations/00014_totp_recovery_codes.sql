-- TOTP recovery codes: single-use account recovery, non-tenant scoped (P2-D8).
--
-- A row belongs to a user, not a shelter, so this table follows refresh_tokens
-- into the auth door rather than the tenant one: RLS enabled and FORCEd, one
-- policy keyed on app.user_id, reachable only through app_auth.
--
-- code_hash is SHA-256, not Argon2id. A password stretcher exists to
-- compensate for low entropy at human-chosen-secret sizes; a recovery code
-- is 128 bits of crypto/rand and there is nothing here to compensate for.
--
-- ENABLE and FORCE land in this same migration as the table and its policy,
-- so there is no intermediate version where the table exists unprotected --
-- unlike refresh_tokens (RLS since 00002, policy only from 00013), this table
-- never needs an entry in rlstest.Schema.PolicyLandsAt.

-- +goose Up

-- +goose StatementBegin
CREATE TABLE totp_recovery_codes (
    id         uuid        NOT NULL PRIMARY KEY,
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,

    -- The lookup key at redemption time. UNIQUE for the same reason
    -- refresh_tokens.token_hash is: two live rows resolving to one hash would
    -- mean two different codes collided or one code was issued twice.
    code_hash  bytea       NOT NULL UNIQUE,

    -- Null means unredeemed. Set once, at redemption, and never cleared --
    -- the single-use guarantee is "the second UPDATE affects zero rows",
    -- checked against this column, not a delete.
    used_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

ALTER TABLE totp_recovery_codes ENABLE ROW LEVEL SECURITY;
ALTER TABLE totp_recovery_codes FORCE  ROW LEVEL SECURITY;

-- WITH CHECK repeats USING, same reason as auth_own_sessions in 00013: USING
-- filters what a statement can see, WITH CHECK is what refuses a forged
-- write naming another user's id.
CREATE POLICY auth_own_recovery_codes ON totp_recovery_codes FOR ALL TO app_auth
    USING      (user_id = nullif(current_setting('app.user_id', true), '')::uuid)
    WITH CHECK (user_id = nullif(current_setting('app.user_id', true), '')::uuid);

-- Table-wide SELECT/INSERT/DELETE, plus a COLUMN grant restricted to used_at
-- for UPDATE (P2-D8): app_auth can regenerate the whole set (DELETE then ten
-- INSERTs) and redeem a code (UPDATE used_at), and can never rewrite
-- code_hash or created_at once a row exists.
GRANT SELECT, INSERT, DELETE ON totp_recovery_codes TO app_auth;
GRANT UPDATE (used_at) ON totp_recovery_codes TO app_auth;

-- +goose Down

REVOKE UPDATE (used_at) ON totp_recovery_codes FROM app_auth;
REVOKE SELECT, INSERT, DELETE ON totp_recovery_codes FROM app_auth;
DROP POLICY IF EXISTS auth_own_recovery_codes ON totp_recovery_codes;
DROP TABLE totp_recovery_codes;
