-- name: RackList :many
SELECT id, rack_row_id, name, total_units, position_in_row, created
FROM dcim.racks
WHERE deleted IS NULL
  AND (sqlc.narg('rack_row_id')::uuid IS NULL OR rack_row_id = sqlc.narg('rack_row_id')::uuid)
ORDER BY created;

-- name: RackGetByID :one
SELECT id, rack_row_id, name, total_units, position_in_row, created
FROM dcim.racks
WHERE id = $1 AND deleted IS NULL;

-- name: RackCreate :one
INSERT INTO dcim.racks (rack_row_id, name, total_units, position_in_row)
VALUES ($1, $2, $3, $4)
RETURNING id;

-- name: RackUpdate :execrows
UPDATE dcim.racks
SET name            = COALESCE(sqlc.narg('name'), name),
    total_units     = COALESCE(sqlc.narg('total_units'), total_units),
    position_in_row = COALESCE(sqlc.narg('position_in_row'), position_in_row)
WHERE id = $1 AND deleted IS NULL;

-- name: RackDelete :execrows
UPDATE dcim.racks
SET deleted = now()
WHERE id = $1 AND deleted IS NULL;
