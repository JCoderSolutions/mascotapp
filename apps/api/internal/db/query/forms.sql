-- Dynamic forms: templates, immutable versions, and recorded answers.

-- name: GetTemplateByKey :one
-- `key` is unique PER SHELTER, so under RLS this resolves to exactly one row
-- without naming the shelter.
SELECT * FROM form_templates WHERE key = $1 AND is_active;

-- name: CreateTemplate :one
INSERT INTO form_templates (id, shelter_id, key, name, purpose)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: PublishTemplateVersion :one
-- Publishing APPENDS. There is deliberately no query that updates a published
-- version: 00007's trigger refuses it, and a query that tried would be code
-- written to be rejected (§4.4 rule 1).
INSERT INTO form_template_versions (id, shelter_id, template_id, version,
                                    definition, published_at, created_by)
VALUES ($1, $2, $3, $4, $5, now(), $6)
RETURNING *;

-- name: GetLatestPublishedVersion :one
SELECT * FROM form_template_versions
WHERE template_id = $1 AND published_at IS NOT NULL
ORDER BY version DESC
LIMIT 1;

-- name: GetVersionForSubmission :one
-- The renderer's path, and the reason historical answers stay readable: it
-- resolves the version the submission was FILLED WITH, never the latest one.
SELECT v.*
FROM form_submissions s
JOIN form_template_versions v ON v.id = s.template_version_id
WHERE s.id = $1;

-- name: RecordSubmission :one
INSERT INTO form_submissions (id, shelter_id, template_version_id, application_id,
                              submitted_by_user_id, answers, ip_hash)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: SearchSubmissions :many
-- Containment, which is what the GIN index on `answers` serves. `answers::text
-- LIKE` would read the same and use no index at all (§4.6).
SELECT * FROM form_submissions
WHERE answers @> $1
ORDER BY submitted_at DESC;
