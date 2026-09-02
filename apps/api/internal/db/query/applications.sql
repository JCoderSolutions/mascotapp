-- The adoption flow: the case, its append-only timeline, and its notes.

-- name: CreateApplication :one
INSERT INTO adoption_applications (id, shelter_id, pet_id, applicant_user_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetApplication :one
SELECT * FROM adoption_applications WHERE id = $1;

-- name: ListApplicationsByStatus :many
-- The queue a refuge works from, ordered to match the §4.6 index
-- (shelter_id, status, created_at DESC).
SELECT * FROM adoption_applications
WHERE status = $1
ORDER BY created_at DESC;

-- name: SetApplicationStatus :execrows
-- WHICH transitions are legal is decided in the domain layer, not here (LT-4).
-- This query moves the status it is given; the caller is what decides whether
-- the move was allowed.
UPDATE adoption_applications
SET status            = $2,
    status_changed_at = now(),
    decision_note     = $3,
    updated_at        = now()
WHERE id = $1;

-- name: AssignApplication :execrows
-- The assignee must hold a membership in this shelter — enforced by the
-- composite key to `memberships (user_id, shelter_id)`, not by this query.
UPDATE adoption_applications
SET assigned_to_user_id = $2, updated_at = now()
WHERE id = $1;

-- name: AppendApplicationEvent :exec
-- Append-only: there is no update or delete counterpart, because there is no
-- grant, no policy and no trigger that would allow one.
INSERT INTO application_events (id, shelter_id, application_id, type, payload, actor_user_id)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListApplicationTimeline :many
SELECT * FROM application_events WHERE application_id = $1 ORDER BY occurred_at;

-- name: WriteApplicationNote :one
INSERT INTO application_notes (id, shelter_id, application_id, author_user_id, body, visibility)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: EditApplicationNote :execrows
-- A note IS editable, unlike an event. That contrast is the design of 00010.
UPDATE application_notes
SET body = $2, updated_at = now()
WHERE id = $1;
