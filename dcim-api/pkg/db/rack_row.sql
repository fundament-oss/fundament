-- name: RackRowList :many
SELECT id, room_id, name, position_x, position_y, created
FROM dcim.rack_rows
WHERE deleted IS NULL
  AND (sqlc.narg('room_id')::uuid IS NULL OR room_id = sqlc.narg('room_id')::uuid)
ORDER BY created;

-- name: RackRowGetByID :one
SELECT id, room_id, name, position_x, position_y, created
FROM dcim.rack_rows
WHERE id = $1 AND deleted IS NULL;

-- name: RackRowCreate :one
INSERT INTO dcim.rack_rows (room_id, name, position_x, position_y)
VALUES ($1, $2, $3, $4)
RETURNING id;

-- name: RackRowUpdate :execrows
UPDATE dcim.rack_rows
SET name       = COALESCE(sqlc.narg('name'), name),
    position_x = COALESCE(sqlc.narg('position_x'), position_x),
    position_y = COALESCE(sqlc.narg('position_y'), position_y)
WHERE id = $1 AND deleted IS NULL;

-- name: RackRowDelete :execrows
UPDATE dcim.rack_rows
SET deleted = now()
WHERE id = $1 AND deleted IS NULL;
