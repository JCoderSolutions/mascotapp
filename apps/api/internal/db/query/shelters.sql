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
