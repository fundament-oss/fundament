-- name: OrganizationCreate :one
INSERT INTO tenant.organizations (name)
VALUES ($1)
RETURNING
  id,
  name,
  created;

-- name: OrganizationList :many
SELECT
  id,
  name,
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

-- name: OrganizationUpdate :one
UPDATE tenant.organizations
SET name = $2
WHERE name = $1
RETURNING id, name, created;
