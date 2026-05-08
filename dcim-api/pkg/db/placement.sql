-- name: PlacementGetByID :one
SELECT id, asset_id, rack_id, start_unit, slot_type, parent_placement_id, port_definition_id, logical_device_id, external_ref, notes, created
FROM dcim.placements
WHERE id = $1 AND deleted IS NULL;

-- name: PlacementCreate :one
INSERT INTO dcim.placements (asset_id, rack_id, start_unit, slot_type, parent_placement_id, port_definition_id, logical_device_id, external_ref, notes)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id;

-- name: PlacementUpdate :execrows
UPDATE dcim.placements
SET rack_id              = COALESCE(sqlc.narg('rack_id'), rack_id),
    start_unit           = COALESCE(sqlc.narg('start_unit'), start_unit),
    slot_type            = COALESCE(sqlc.narg('slot_type'), slot_type),
    parent_placement_id  = COALESCE(sqlc.narg('parent_placement_id'), parent_placement_id),
    port_definition_id   = COALESCE(sqlc.narg('port_definition_id'), port_definition_id),
    logical_device_id    = COALESCE(sqlc.narg('logical_device_id'), logical_device_id),
    notes                = COALESCE(sqlc.narg('notes'), notes)
WHERE id = $1 AND deleted IS NULL;

-- name: PlacementDelete :execrows
UPDATE dcim.placements
SET deleted = now()
WHERE id = $1 AND deleted IS NULL;

-- name: PlacementListByRack :many
SELECT id, asset_id, rack_id, start_unit, slot_type, parent_placement_id, port_definition_id, logical_device_id, external_ref, notes, created
FROM dcim.placements
WHERE rack_id = $1 AND deleted IS NULL
ORDER BY start_unit;

-- name: PlacementListByParent :many
SELECT id, asset_id, rack_id, start_unit, slot_type, parent_placement_id, port_definition_id, logical_device_id, external_ref, notes, created
FROM dcim.placements
WHERE parent_placement_id = $1 AND deleted IS NULL
ORDER BY created;
