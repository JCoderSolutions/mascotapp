-- The forensic trail.

-- name: AppendAuditEntry :one
-- NO identifier is supplied: `id` is a `bigserial`, and letting the sequence
-- issue it is what requires `GRANT USAGE ON SEQUENCE` (T-01-032). A query that
-- passed an id would work in every test and fail on the first real write.
INSERT INTO audit_log (shelter_id, actor_user_id, action, entity_type, entity_id,
                       "before", "after", ip_hash, user_agent)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id;

-- name: ListRecentAuditEntries :many
-- Ordered to match the §4.6 index (shelter_id, occurred_at DESC).
SELECT * FROM audit_log ORDER BY occurred_at DESC LIMIT $1;

-- name: ListAuditEntriesForEntity :many
-- `entity_type` and `entity_id` are a polymorphic pair with no foreign key, so
-- BOTH are needed: an id alone could collide across tables.
SELECT * FROM audit_log
WHERE entity_type = $1 AND entity_id = $2
ORDER BY occurred_at DESC;
