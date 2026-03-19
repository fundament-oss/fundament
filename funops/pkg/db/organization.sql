-- name: OrganizationCreate :one
INSERT INTO tenant.organizations (name, alias)
VALUES ($1, $2)
RETURNING
  id,
  name,
  alias,
  created;

-- name: OrganizationList :many
SELECT
  id,
  name,
  alias,
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
