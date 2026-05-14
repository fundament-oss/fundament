-- name: LogicalConnectionList :many
SELECT id, logical_design_id, a_logical_device_id, a_port_role, b_logical_device_id, b_port_role, connection_type, requirements, label, created
FROM dcim.logical_connections
WHERE logical_design_id = $1 AND deleted IS NULL
ORDER BY created;

-- name: LogicalConnectionGetByID :one
SELECT id, logical_design_id, a_logical_device_id, a_port_role, b_logical_device_id, b_port_role, connection_type, requirements, label, created
FROM dcim.logical_connections
WHERE id = $1 AND deleted IS NULL;

-- name: LogicalConnectionCreate :one
INSERT INTO dcim.logical_connections (logical_design_id, a_logical_device_id, a_port_role, b_logical_device_id, b_port_role, connection_type, requirements, label)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id;

-- name: LogicalConnectionUpdate :execrows
UPDATE dcim.logical_connections
SET a_port_role      = COALESCE(sqlc.narg('a_port_role'), a_port_role),
    b_port_role      = COALESCE(sqlc.narg('b_port_role'), b_port_role),
    connection_type  = COALESCE(sqlc.narg('connection_type'), connection_type),
    requirements     = COALESCE(sqlc.narg('requirements'), requirements),
    label            = COALESCE(sqlc.narg('label'), label)
WHERE id = $1 AND deleted IS NULL;

-- name: LogicalConnectionDelete :execrows
UPDATE dcim.logical_connections
SET deleted = now()
WHERE id = $1 AND deleted IS NULL;
