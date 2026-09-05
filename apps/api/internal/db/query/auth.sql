-- The auth door's own queries: everything reachable only through app_auth
-- (P2-D1..P2-D3, migration 00013). Handlers built later in this phase call
-- these inside db.WithAuthUser / db.WithAuthLookup, never inside WithTenant.
--
-- Same rule as every other file here: NOT ONE of these filters by
-- shelter_id. app_auth cannot even set app.shelter_id -- there is no claim
-- yet at this door -- so a query that tried would just be wrong, not merely
-- redundant with the policy.

-- name: GetRefreshTokenByHash :one
-- The rotation lookup (design's rotate flow): parse the cookie's user_id
-- prefix, WithAuthUser(userID), then find the presented token by its hash
-- inside that scope. token_hash is UNIQUE, so :one is correct.
SELECT * FROM refresh_tokens WHERE token_hash = $1;

-- name: InsertRefreshToken :exec
-- Both halves of a session's life: login starts a family by passing a fresh
-- family_id, rotation continues one by passing the presented token's. The
-- caller chooses, because only the caller knows which of the two it is --
-- a default here would silently make every rotation start a new family and
-- quietly disable family-wide revocation.
INSERT INTO refresh_tokens (id, user_id, token_hash, family_id, expires_at)
VALUES ($1, $2, $3, $4, $5);

-- name: MarkRefreshTokenRotated :execrows
-- The rotation half of the write: the presented token is revoked AND
-- pointed at its successor, in one statement.
--
-- `revoked_at IS NULL` is a guard, not decoration: it makes this the write
-- that loses a concurrent race. Two simultaneous rotations of the same
-- token both read a live row, and only one can affect a row here -- the
-- other gets zero and is refused, instead of both minting a session.
UPDATE refresh_tokens
SET revoked_at = now(), replaced_by = $2
WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokeRefreshTokenFamily :execrows
-- Reuse detection revokes the whole family in one statement (§5.2): every
-- token sharing family_id, not just the one presented. execrows so a caller
-- can tell "revoked N tokens" apart from "the family was already revoked".
UPDATE refresh_tokens
SET revoked_at = now()
WHERE family_id = $1 AND revoked_at IS NULL;

-- name: GetUserCredentialsByEmail :one
-- Only the columns app_auth's SELECT grant carries (P2-D3, 00013). SELECT *
-- would name full_name and phone, which app_auth cannot read at all, and the
-- statement would fail 42501 before a row ever came back.
SELECT id, email, password_hash, status, totp_secret_enc, email_verified_at, created_at
FROM users WHERE email = $1;

-- name: ListOwnMemberships :many
-- Login needs to know which shelters a user belongs to in order to mint a
-- shelter-exchange claim later -- nothing else. Column- and row-scoped to the
-- caller (auth_own_memberships, P2-D3): app_auth cannot read another user's
-- membership row at all, so no predicate here could leak past the policy.
SELECT id, user_id, shelter_id, role, status FROM memberships WHERE user_id = $1;

-- totp_recovery_codes (P2-D8, migration 00014). The regeneration flow
-- (T-02-017/018) issues DeleteRecoveryCodesForUser once and InsertRecoveryCode
-- ten times inside one WithAuthUser transaction; SELECT/DELETE carry a table
-- grant, INSERT the same, and UPDATE is column-scoped to used_at only -- code_hash
-- and created_at are never rewritten once a row exists.

-- name: InsertRecoveryCode :exec
INSERT INTO totp_recovery_codes (id, user_id, code_hash) VALUES ($1, $2, $3);

-- name: DeleteRecoveryCodesForUser :execrows
-- Regeneration starts by clearing the previous set, all ten in one
-- statement, in the same transaction as the ten inserts above.
DELETE FROM totp_recovery_codes WHERE user_id = $1;

-- name: GetRecoveryCodeByHash :one
-- The redemption lookup: parse the submitted code, hash it, find the row.
-- code_hash is UNIQUE, so :one is correct.
SELECT * FROM totp_recovery_codes WHERE code_hash = $1;

-- name: RedeemRecoveryCode :execrows
-- Single-use redemption: only a row with used_at still null is matched, so a
-- repeated or concurrent redemption of the same code affects zero rows --
-- the same "qualified write, zero rows means refused" shape
-- RevokeRefreshTokenFamily above uses for its own idempotency question.
UPDATE totp_recovery_codes
SET used_at = now()
WHERE code_hash = $1 AND used_at IS NULL;
