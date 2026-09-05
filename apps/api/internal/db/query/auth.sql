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

-- name: LockRefreshTokenFamily :exec
-- Serialises every transaction that touches one session family.
--
-- This exists because READ COMMITTED alone cannot give the family-revocation
-- guarantee. `RevokeRefreshTokenFamily` below fixes its candidate rows at the
-- snapshot its own statement takes; a successor row that a CONCURRENT rotation
-- inserts afterwards is never in that set and survives the revocation of its
-- own family -- permanently, because nothing revokes an already-revoked family
-- a second time. Judgment Day (T-02-021) found it and a concurrency test
-- reproduces it.
--
-- Taken after the presented row is read, because the family_id is not known
-- before that. Once it is held, no other transaction can begin deciding this
-- family until this one ends, so a concurrent revocation either already
-- committed (and RevokeRefreshTokenIfLive below then matches zero rows and the
-- rotation is refused) or cannot start until the successor row is committed and
-- therefore visible to it.
--
-- `xact` means it releases at COMMIT or ROLLBACK, so no path can leak it.
--
-- A hash collision between two different family_ids costs concurrency, never
-- correctness: the two families would serialise against each other for no
-- reason, and both still behave correctly.
SELECT pg_advisory_xact_lock(hashtext($1::text)::bigint);

-- name: RevokeRefreshTokenIfLive :execrows
-- The FIRST half of a rotation's write, and now the only one that revokes
-- anything (00017 split what used to be one statement, MarkRefreshTokenRotated,
-- into this and SetRefreshTokenReplacedBy below).
--
-- This runs BEFORE the successor is inserted, not after: 00017 adds a unique
-- index enforcing at most one live row per family_id, and the old
-- insert-then-mark order would transiently hold the presented token AND its
-- successor live at once, which that index now refuses.
--
-- `revoked_at IS NULL` is a guard, not decoration: it makes this the write
-- that loses a concurrent race. Two simultaneous rotations of the same
-- token both read a live row, and only one can affect a row here -- the
-- other gets zero and is refused, instead of both minting a session.
UPDATE refresh_tokens
SET revoked_at = now()
WHERE id = $1 AND revoked_at IS NULL;

-- name: SetRefreshTokenReplacedBy :exec
-- The SECOND half: the back-pointer can only be set once the successor row
-- exists, because replaced_by is a foreign key into this same table. Split
-- off from the revoke above so the revoke can run first -- see the note
-- there.
UPDATE refresh_tokens
SET replaced_by = $2
WHERE id = $1;

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
