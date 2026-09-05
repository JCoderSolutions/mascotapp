-- The database backstop for "a family has at most one live refresh token"
-- (Judgment Day, PR-02-11): closing a rotation race that the application
-- layer alone could not close.
--
-- `RevokeRefreshTokenFamily` (00013) is a single
-- `UPDATE ... WHERE family_id = $1 AND revoked_at IS NULL`, run under READ
-- COMMITTED. An UPDATE's candidate rows are fixed at the STATEMENT'S
-- snapshot: a successor row inserted by a concurrent rotation, if it did not
-- exist yet when that snapshot was taken, is never a candidate for this
-- statement and is not revoked by it. Nothing else ever revokes it
-- afterwards -- the family already reads as "revoked" -- so a token
-- survives the exact event meant to kill its whole family.
--
-- This index makes the invariant the application was trusting into
-- something the database enforces: at most one row per family_id may have
-- `revoked_at IS NULL` at any committed instant. A rotation that tried to
-- hold two live rows in one family at once -- even transiently, even inside
-- its own transaction -- now fails the constraint instead of committing.
-- That is why `RotateRefreshToken` revokes the presented token BEFORE
-- inserting its successor (session.go): the old insert-then-mark order would
-- transiently hold both rows live and this index would reject it.
--
-- Partial, not a plain UNIQUE(family_id): a family accumulates many
-- REVOKED rows over its life -- one per rotation -- and only the single
-- live one is what this constraint is about.

-- +goose Up

CREATE UNIQUE INDEX one_live_token_per_family
    ON refresh_tokens (family_id)
    WHERE revoked_at IS NULL;

-- +goose Down

DROP INDEX one_live_token_per_family;
