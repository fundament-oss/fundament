
-- name: OrganizationGetByID :one
SELECT id, name, created
FROM tenant.organizations
WHERE id = $1;

-- name: OrganizationUpdate :one
UPDATE tenant.organizations
SET name = $2
WHERE id = $1
RETURNING id, name, created;
