-- name: PhysicalConnectionGetByID :one
SELECT id, a_placement_id, a_port_definition_id, b_placement_id, b_port_definition_id, cable_asset_id, logical_connection_id, cable_type, status, color, label, created
FROM dcim.physical_connections
WHERE id = $1 AND deleted IS NULL;

-- name: PhysicalConnectionCreate :one
INSERT INTO dcim.physical_connections (a_placement_id, a_port_definition_id, b_placement_id, b_port_definition_id, cable_asset_id, logical_connection_id, cable_type, status, color, label)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id;

-- name: PhysicalConnectionUpdate :execrows
UPDATE dcim.physical_connections
SET cable_asset_id        = CASE
        WHEN sqlc.arg('clear_cable_asset_id')::bool THEN NULL
        ELSE COALESCE(sqlc.narg('cable_asset_id'), cable_asset_id)
    END,
    logical_connection_id = CASE
        WHEN sqlc.arg('clear_logical_connection_id')::bool THEN NULL
        ELSE COALESCE(sqlc.narg('logical_connection_id'), logical_connection_id)
    END,
    cable_type = CASE
        WHEN sqlc.arg('clear_cable_type')::bool THEN NULL
        ELSE COALESCE(sqlc.narg('cable_type'), cable_type)
    END,
    status     = CASE
        WHEN sqlc.arg('clear_status')::bool THEN NULL
        ELSE COALESCE(sqlc.narg('status'), status)
    END,
    color      = CASE
        WHEN sqlc.arg('clear_color')::bool THEN NULL
        ELSE COALESCE(sqlc.narg('color'), color)
    END,
    label      = CASE
        WHEN sqlc.arg('clear_label')::bool THEN NULL
        ELSE COALESCE(sqlc.narg('label'), label)
    END
WHERE id = $1 AND deleted IS NULL;

-- name: PhysicalConnectionDelete :execrows
UPDATE dcim.physical_connections
SET deleted = now()
WHERE id = $1 AND deleted IS NULL;

-- name: PhysicalConnectionListByPlacement :many
SELECT id, a_placement_id, a_port_definition_id, b_placement_id, b_port_definition_id, cable_asset_id, logical_connection_id, cable_type, status, color, label, created
FROM dcim.physical_connections
WHERE (a_placement_id = $1 OR b_placement_id = $1) AND deleted IS NULL
ORDER BY created;

-- name: PhysicalConnectionListBySite :many
-- Every connection whose a- or b-side placement sits in a rack belonging to the
-- given site. Resolved through placement -> rack -> rack_row -> room -> site,
-- skipping any soft-deleted link in that chain.
WITH site_placements AS (
    SELECT dcim.placements.id
    FROM dcim.placements
    JOIN dcim.racks ON dcim.racks.id = dcim.placements.rack_id AND dcim.racks.deleted IS NULL
    JOIN dcim.rack_rows ON dcim.rack_rows.id = dcim.racks.rack_row_id AND dcim.rack_rows.deleted IS NULL
    JOIN dcim.rooms ON dcim.rooms.id = dcim.rack_rows.room_id AND dcim.rooms.deleted IS NULL
    WHERE dcim.placements.deleted IS NULL
      AND dcim.rooms.site_id = $1
)
SELECT id, a_placement_id, a_port_definition_id, b_placement_id, b_port_definition_id, cable_asset_id, logical_connection_id, cable_type, status, color, label, created
FROM dcim.physical_connections
WHERE deleted IS NULL
  AND (a_placement_id IN (SELECT site_placements.id FROM site_placements)
       OR b_placement_id IN (SELECT site_placements.id FROM site_placements))
ORDER BY created;
