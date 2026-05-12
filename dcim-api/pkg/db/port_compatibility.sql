-- name: PortCompatibilityList :many
SELECT id, port_definition_id, compatible_category, compatible_catalog_id, created
FROM dcim.port_compatibilities
WHERE deleted IS NULL
  AND port_definition_id = $1
ORDER BY created;

-- name: PortCompatibilityCreate :one
INSERT INTO dcim.port_compatibilities (port_definition_id, compatible_category, compatible_catalog_id)
SELECT sqlc.arg('port_definition_id')::uuid, category, sqlc.arg('compatible_catalog_id')::uuid
FROM dcim.device_catalogs
WHERE id = sqlc.arg('compatible_catalog_id')::uuid AND deleted IS NULL
RETURNING id;

-- name: PortCompatibilityDelete :execrows
UPDATE dcim.port_compatibilities
SET deleted = now()
WHERE port_definition_id = $1
  AND compatible_catalog_id = $2
  AND deleted IS NULL;
