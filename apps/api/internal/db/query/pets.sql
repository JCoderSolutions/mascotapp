-- The catalog core and its three children.

-- name: CreatePet :one
INSERT INTO pets (id, shelter_id, public_code, name, species_id, breed_id, sex, size,
                  energy_level, good_with_kids, good_with_dogs, good_with_cats,
                  description, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING *;

-- name: GetPet :one
SELECT * FROM pets WHERE id = $1 AND deleted_at IS NULL;

-- name: ListPetsByStatus :many
-- The shelter dashboard read, ordered to match the §4.6 index
-- (shelter_id, status, published_at DESC) so the plan can walk it.
SELECT * FROM pets
WHERE status = $1 AND deleted_at IS NULL
ORDER BY published_at DESC NULLS LAST;

-- name: PublishPet :execrows
UPDATE pets
SET status            = 'available',
    published_at      = coalesce(published_at, now()),
    status_changed_at = now(),
    updated_at        = now()
WHERE id = $1 AND deleted_at IS NULL;

-- name: AttachPhoto :exec
INSERT INTO pet_media (pet_id, media_id, shelter_id, position, is_primary)
VALUES ($1, $2, $3, $4, $5);

-- name: RecordHealthEvent :one
INSERT INTO pet_health_records (id, shelter_id, pet_id, type, occurred_on,
                                description, vet_name, document_media_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: AppendStatusChange :exec
-- `pet_status_history` is append-only (LT-5): insert and select, nothing else.
-- There is no update or delete query here because there is no grant for one.
INSERT INTO pet_status_history (id, shelter_id, pet_id, from_status, to_status,
                                reason, actor_user_id)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListStatusHistory :many
SELECT * FROM pet_status_history WHERE pet_id = $1 ORDER BY occurred_at DESC;
