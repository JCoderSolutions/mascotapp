-- Tenancy and identity.
--
-- NOT ONE of these reads filters by `shelter_id`, and that is deliberate rather
-- than an omission: the policy does it. A query that filtered too would return
-- the same rows whether or not the policy existed, which is how the loss of a
-- policy becomes invisible (§3 layer 3).

-- name: GetCurrentShelter :one
-- The tenant's own row. No predicate at all: under RLS this is the only row
-- `shelters` has for this transaction, which is the property the A/B suite pins.
SELECT * FROM shelters LIMIT 1;

-- name: UpdateShelterProfile :execrows
-- The institutional profile of §4.1. `execrows` rather than `exec`: a policy
-- that filtered the row away reports zero rows, and callers have to be able to
-- tell that apart from a successful write.
UPDATE shelters
SET display_name = $2,
    mission      = $3,
    vision       = $4,
    about        = $5,
    updated_at   = now()
WHERE id = $1;

-- name: CreateShelter :exec
-- Registration's second transaction (P2-D6), as `app_tenant` inside WithTenant.
--
-- `status` is absent on purpose, and its absence is enforced twice over: the
-- column defaults to 'pending_verification' (LT-2), and 00015 does not grant
-- `app_tenant` INSERT on it at all. A caller who submits a status cannot reach
-- this statement with it, and a statement that named it would be refused by the
-- grant. The handler drops it at the edge because an API that silently ignores
-- a field is friendlier than one that 500s -- but the database is what makes
-- that a guarantee rather than a habit.
--
-- `id` is supplied rather than defaulted because WITH CHECK compares it against
-- app.shelter_id, which the caller has already set on this transaction.
INSERT INTO shelters (id, slug, display_name, legal_name, country, state, city,
                      contact_email)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);
