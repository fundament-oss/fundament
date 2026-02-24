-- name: OrganizationCreate :one
INSERT INTO tenant.organizations (name, display_name)
VALUES ($1, $2)
RETURNING
  id,
  name,
  display_name,
  created;

-- name: OrganizationList :many
SELECT
  id,
  name,
  display_name,
  created
FROM tenant.organizations
ORDER BY created DESC;

-- name: OrganizationDelete :execrows
DELETE FROM tenant.organizations
WHERE name = $1;

-- name: OrganizationGetIDByName :one
SELECT id
FROM tenant.organizations
WHERE name = $1;
