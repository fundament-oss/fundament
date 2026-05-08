-- name: PortCompatibilityList :many
SELECT id, port_definition_id, compatible_category, compatible_catalog_id, created
FROM dcim.port_compatibilities
WHERE deleted IS NULL
  AND port_definition_id = $1
ORDER BY created;

-- name: PortCompatibilityCreate :one
INSERT INTO dcim.port_compatibilities (port_definition_id, compatible_category, compatible_catalog_id)
VALUES ($1, $2, $3)
RETURNING id;

-- name: PortCompatibilityDelete :execrows
UPDATE dcim.port_compatibilities
SET deleted = now()
WHERE port_definition_id = $1
  AND compatible_catalog_id = $2
  AND deleted IS NULL;
