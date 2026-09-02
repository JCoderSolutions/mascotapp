-- Users and memberships.
--
-- `users` is the one table in this schema that belongs to no shelter (D6), so
-- its visibility comes from a policy that reads `memberships` and, since 00009,
-- `adoption_applications`. A caller sees exactly the users those two branches
-- allow — nothing here narrows it further.

-- name: GetUserByEmail :one
-- `email` is `citext`, so this finds a differently-cased address without the
-- call site remembering to normalise (D7).
SELECT * FROM users WHERE email = $1;

-- name: ListShelterMembers :many
-- The staff of the current shelter. `memberships` is tenant-scoped, so the join
-- is already narrowed to this tenant without a predicate here.
SELECT sqlc.embed(users), m.role, m.status
FROM memberships m
JOIN users ON users.id = m.user_id
ORDER BY users.full_name;

-- name: InviteMember :exec
-- Supplying `shelter_id` is not filtering by it: the column is NOT NULL and
-- `WITH CHECK` is what verifies the value.
INSERT INTO memberships (id, user_id, shelter_id, role, status, invited_by)
VALUES ($1, $2, $3, $4, 'invited', $5);
