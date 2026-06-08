-- name: NoteList :many
SELECT id, body, created_by, device_catalog_id, port_definition_id, asset_id, site_id, room_id, rack_row_id, rack_id, placement_id, physical_connection_id, logical_design_id, logical_device_id, logical_connection_id, task_id, created
FROM dcim.notes
WHERE deleted IS NULL
  AND (
    (sqlc.narg('device_catalog_id')::uuid IS NOT NULL AND device_catalog_id = sqlc.narg('device_catalog_id')::uuid) OR
    (sqlc.narg('port_definition_id')::uuid IS NOT NULL AND port_definition_id = sqlc.narg('port_definition_id')::uuid) OR
    (sqlc.narg('asset_id')::uuid IS NOT NULL AND asset_id = sqlc.narg('asset_id')::uuid) OR
    (sqlc.narg('site_id')::uuid IS NOT NULL AND site_id = sqlc.narg('site_id')::uuid) OR
    (sqlc.narg('room_id')::uuid IS NOT NULL AND room_id = sqlc.narg('room_id')::uuid) OR
    (sqlc.narg('rack_row_id')::uuid IS NOT NULL AND rack_row_id = sqlc.narg('rack_row_id')::uuid) OR
    (sqlc.narg('rack_id')::uuid IS NOT NULL AND rack_id = sqlc.narg('rack_id')::uuid) OR
    (sqlc.narg('placement_id')::uuid IS NOT NULL AND placement_id = sqlc.narg('placement_id')::uuid) OR
    (sqlc.narg('physical_connection_id')::uuid IS NOT NULL AND physical_connection_id = sqlc.narg('physical_connection_id')::uuid) OR
    (sqlc.narg('logical_design_id')::uuid IS NOT NULL AND logical_design_id = sqlc.narg('logical_design_id')::uuid) OR
    (sqlc.narg('logical_device_id')::uuid IS NOT NULL AND logical_device_id = sqlc.narg('logical_device_id')::uuid) OR
    (sqlc.narg('logical_connection_id')::uuid IS NOT NULL AND logical_connection_id = sqlc.narg('logical_connection_id')::uuid) OR
    (sqlc.narg('task_id')::uuid IS NOT NULL AND task_id = sqlc.narg('task_id')::uuid)
  )
ORDER BY created;

-- name: NoteCreate :one
INSERT INTO dcim.notes (body, created_by, device_catalog_id, port_definition_id, asset_id, site_id, room_id, rack_row_id, rack_id, placement_id, physical_connection_id, logical_design_id, logical_device_id, logical_connection_id, task_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING id;

-- name: NoteDelete :execrows
UPDATE dcim.notes
SET deleted = now()
WHERE id = $1 AND deleted IS NULL;
