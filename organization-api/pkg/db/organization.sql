
-- name: OrganizationGetByID :one
SELECT id, name, alias, created
FROM tenant.organizations
WHERE id = $1;

-- name: OrganizationUpdate :one
UPDATE tenant.organizations
SET alias = $2
WHERE id = $1
RETURNING id, name, alias, created;

-- name: OrganizationList :many
SELECT id, name, alias, created
FROM tenant.organizations
ORDER BY created;
