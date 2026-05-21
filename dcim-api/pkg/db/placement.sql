-- name: PlacementGetByID :one
SELECT id, asset_id, rack_id, start_unit, slot_type, parent_placement_id, port_definition_id, logical_device_id, external_ref, notes, created
FROM dcim.placements
WHERE id = $1 AND deleted IS NULL;

-- name: PlacementGetByAsset :one
SELECT id, asset_id, rack_id, start_unit, slot_type, parent_placement_id, port_definition_id, logical_device_id, external_ref, notes, created
FROM dcim.placements
WHERE asset_id = $1 AND deleted IS NULL;

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
    logical_device_id    = CASE
        WHEN sqlc.arg('clear_logical_device_id')::bool THEN NULL
        ELSE COALESCE(sqlc.narg('logical_device_id'), logical_device_id)
    END,
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

-- name: PlacementResolveRackByAsset :one
-- Walks parent_placement_id up from the asset's placement so nested
-- sub-components resolve the rack of their top-level host.
WITH RECURSIVE location_chain AS (
    SELECT dcim.placements.id, dcim.placements.rack_id, dcim.placements.start_unit, dcim.placements.slot_type, dcim.placements.parent_placement_id
    FROM dcim.placements
    WHERE dcim.placements.asset_id = $1 AND dcim.placements.deleted IS NULL
    UNION ALL
    SELECT dcim.placements.id, dcim.placements.rack_id, dcim.placements.start_unit, dcim.placements.slot_type, dcim.placements.parent_placement_id
    FROM dcim.placements
    JOIN location_chain ON dcim.placements.id = location_chain.parent_placement_id
    WHERE dcim.placements.deleted IS NULL
)
SELECT location_chain.rack_id, location_chain.start_unit, location_chain.slot_type
FROM location_chain
WHERE location_chain.rack_id IS NOT NULL;
