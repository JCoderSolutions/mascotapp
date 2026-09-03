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
