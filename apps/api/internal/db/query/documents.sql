-- Contracts, receipts and certificates.

-- name: FileDocument :one
-- `application_id` may be null: a health certificate is filed against an animal
-- long before anybody applies for it.
INSERT INTO documents (id, shelter_id, media_id, application_id, type, generated_from)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListApplicationDocuments :many
SELECT * FROM documents WHERE application_id = $1 ORDER BY created_at DESC;

-- name: SignDocument :execrows
UPDATE documents SET signed_at = now(), signature = $2 WHERE id = $1;
