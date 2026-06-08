-- name: PortDefinitionList :many
SELECT id, device_catalog_id, name, port_type, media_type, speed, max_power_w, direction, ordinal, created
FROM dcim.port_definitions
WHERE deleted IS NULL
  AND device_catalog_id = $1
ORDER BY ordinal;

-- name: PortDefinitionGetByID :one
SELECT id, device_catalog_id, name, port_type, media_type, speed, max_power_w, direction, ordinal, created
FROM dcim.port_definitions
WHERE id = $1 AND deleted IS NULL;

-- name: PortDefinitionCreate :one
INSERT INTO dcim.port_definitions (device_catalog_id, name, port_type, media_type, speed, max_power_w, direction, ordinal)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id;

-- name: PortDefinitionUpdate :execrows
UPDATE dcim.port_definitions
SET name        = COALESCE(sqlc.narg('name'), name),
    port_type   = COALESCE(sqlc.narg('port_type'), port_type),
    media_type  = COALESCE(sqlc.narg('media_type'), media_type),
    speed       = COALESCE(sqlc.narg('speed'), speed),
    max_power_w = COALESCE(sqlc.narg('max_power_w'), max_power_w),
    direction   = COALESCE(sqlc.narg('direction'), direction),
    ordinal     = COALESCE(sqlc.narg('ordinal'), ordinal)
WHERE id = $1 AND deleted IS NULL;

-- name: PortDefinitionDelete :execrows
UPDATE dcim.port_definitions
SET deleted = now()
WHERE id = $1 AND deleted IS NULL;
