-- name: NoteList :many
-- users.name is the note author's display name, joined via notes.created_by_id.
-- It lands on the generated row as the bare field Name (no aliases allowed), so
-- read that as "author name", not as a name belonging to the note itself.
SELECT notes.id, notes.body, users.name, notes.device_catalog_id, notes.port_definition_id, notes.asset_id, notes.site_id, notes.room_id, notes.rack_row_id, notes.rack_id, notes.placement_id, notes.physical_connection_id, notes.logical_design_id, notes.logical_device_id, notes.logical_connection_id, notes.task_id, notes.created
FROM dcim.notes
LEFT JOIN dcim.users ON users.id = notes.created_by_id
WHERE notes.deleted IS NULL
  AND (
    (sqlc.narg('device_catalog_id')::uuid IS NOT NULL AND notes.device_catalog_id = sqlc.narg('device_catalog_id')::uuid) OR
    (sqlc.narg('port_definition_id')::uuid IS NOT NULL AND notes.port_definition_id = sqlc.narg('port_definition_id')::uuid) OR
    (sqlc.narg('asset_id')::uuid IS NOT NULL AND notes.asset_id = sqlc.narg('asset_id')::uuid) OR
    (sqlc.narg('site_id')::uuid IS NOT NULL AND notes.site_id = sqlc.narg('site_id')::uuid) OR
    (sqlc.narg('room_id')::uuid IS NOT NULL AND notes.room_id = sqlc.narg('room_id')::uuid) OR
    (sqlc.narg('rack_row_id')::uuid IS NOT NULL AND notes.rack_row_id = sqlc.narg('rack_row_id')::uuid) OR
    (sqlc.narg('rack_id')::uuid IS NOT NULL AND notes.rack_id = sqlc.narg('rack_id')::uuid) OR
    (sqlc.narg('placement_id')::uuid IS NOT NULL AND notes.placement_id = sqlc.narg('placement_id')::uuid) OR
    (sqlc.narg('physical_connection_id')::uuid IS NOT NULL AND notes.physical_connection_id = sqlc.narg('physical_connection_id')::uuid) OR
    (sqlc.narg('logical_design_id')::uuid IS NOT NULL AND notes.logical_design_id = sqlc.narg('logical_design_id')::uuid) OR
    (sqlc.narg('logical_device_id')::uuid IS NOT NULL AND notes.logical_device_id = sqlc.narg('logical_device_id')::uuid) OR
    (sqlc.narg('logical_connection_id')::uuid IS NOT NULL AND notes.logical_connection_id = sqlc.narg('logical_connection_id')::uuid) OR
    (sqlc.narg('task_id')::uuid IS NOT NULL AND notes.task_id = sqlc.narg('task_id')::uuid)
  )
ORDER BY notes.created;

-- name: NoteCreate :one
INSERT INTO dcim.notes (body, created_by_id, device_catalog_id, port_definition_id, asset_id, site_id, room_id, rack_row_id, rack_id, placement_id, physical_connection_id, logical_design_id, logical_device_id, logical_connection_id, task_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING id;

-- name: NoteDelete :execrows
UPDATE dcim.notes
SET deleted = now()
WHERE id = $1 AND deleted IS NULL;
