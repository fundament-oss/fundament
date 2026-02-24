
-- name: OrganizationGetByID :one
SELECT id, name, display_name, created
FROM tenant.organizations
WHERE id = $1;

-- name: OrganizationUpdate :one
UPDATE tenant.organizations
SET display_name = $2
WHERE id = $1
RETURNING id, name, display_name, created;

-- name: OrganizationList :many
SELECT id, name, display_name, created
FROM tenant.organizations
ORDER BY created;
