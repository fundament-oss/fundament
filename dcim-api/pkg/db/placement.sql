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

-- name: PlacementResolveLocationByAsset :one
-- Walks parent_placement_id up from the asset's placement so nested
-- sub-components resolve the rack of their top-level host, then joins
-- the rack hierarchy (rack -> rack_row -> room -> site) to return
-- human-readable names alongside the placement details.
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
SELECT
    location_chain.rack_id,
    location_chain.start_unit,
    location_chain.slot_type,
    dcim.racks.name     AS rack_name,
    dcim.rack_rows.name AS rack_row_name,
    dcim.rooms.name     AS room_name,
    dcim.sites.name     AS site_name
FROM location_chain
JOIN dcim.racks     ON dcim.racks.id     = location_chain.rack_id    AND dcim.racks.deleted     IS NULL
JOIN dcim.rack_rows ON dcim.rack_rows.id = dcim.racks.rack_row_id    AND dcim.rack_rows.deleted IS NULL
JOIN dcim.rooms     ON dcim.rooms.id     = dcim.rack_rows.room_id    AND dcim.rooms.deleted     IS NULL
JOIN dcim.sites     ON dcim.sites.id     = dcim.rooms.site_id        AND dcim.sites.deleted     IS NULL
WHERE location_chain.rack_id IS NOT NULL
LIMIT 1;
