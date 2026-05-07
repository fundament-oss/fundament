-- name: RoomList :many
SELECT id, site_id, name, floor, created
FROM dcim.rooms
WHERE deleted IS NULL
  AND (sqlc.narg('site_id')::uuid IS NULL OR site_id = sqlc.narg('site_id')::uuid)
ORDER BY created;

-- name: RoomGetByID :one
SELECT id, site_id, name, floor, created
FROM dcim.rooms
WHERE id = $1 AND deleted IS NULL;

-- name: RoomCreate :one
INSERT INTO dcim.rooms (site_id, name, floor)
VALUES ($1, $2, $3)
RETURNING id;

-- name: RoomUpdate :execrows
UPDATE dcim.rooms
SET name  = COALESCE(sqlc.narg('name'), name),
    floor = COALESCE(sqlc.narg('floor'), floor)
WHERE id = $1 AND deleted IS NULL;

-- name: RoomDelete :execrows
UPDATE dcim.rooms
SET deleted = now()
WHERE id = $1 AND deleted IS NULL;
