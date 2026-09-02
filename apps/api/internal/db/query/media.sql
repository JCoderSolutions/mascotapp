-- Uploaded files. The bytes live in R2 (§5.5); this is the record of them.

-- name: RecordUpload :one
INSERT INTO media (id, shelter_id, kind, storage_key, mime, bytes,
                   width, height, checksum_sha256, alt_text, uploaded_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetMedia :one
SELECT * FROM media WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteMedia :execrows
-- Soft, because `pet_media` and `documents` reference it with RESTRICT and a
-- hard delete would be refused by whichever one points at it.
UPDATE media SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL;
