-- Global reference data: readable by both roles, writable by neither.

-- name: ListSpecies :many
SELECT * FROM species ORDER BY name;

-- name: ListBreedsForSpecies :many
-- `breeds` carries no `shelter_id` at all, so `species_id` is a domain filter
-- rather than a tenant one.
SELECT * FROM breeds WHERE species_id = $1 ORDER BY name;
