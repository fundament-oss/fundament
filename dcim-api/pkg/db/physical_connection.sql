-- name: PhysicalConnectionGetByID :one
SELECT id, a_placement_id, a_port_definition_id, b_placement_id, b_port_definition_id, cable_asset_id, logical_connection_id, created
FROM dcim.physical_connections
WHERE id = $1 AND deleted IS NULL;

-- name: PhysicalConnectionCreate :one
INSERT INTO dcim.physical_connections (a_placement_id, a_port_definition_id, b_placement_id, b_port_definition_id, cable_asset_id, logical_connection_id)
VALUES ($1, $2, $3, $4, $5, $6)
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
    END
WHERE id = $1 AND deleted IS NULL;

-- name: PhysicalConnectionDelete :execrows
UPDATE dcim.physical_connections
SET deleted = now()
WHERE id = $1 AND deleted IS NULL;

-- name: PhysicalConnectionListByPlacement :many
SELECT id, a_placement_id, a_port_definition_id, b_placement_id, b_port_definition_id, cable_asset_id, logical_connection_id, created
FROM dcim.physical_connections
WHERE (a_placement_id = $1 OR b_placement_id = $1) AND deleted IS NULL
ORDER BY created;
