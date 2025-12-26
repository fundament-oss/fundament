-- name: TenantGetByID :one
SELECT id, name, created
FROM organization.tenants
WHERE id = $1;

-- name: TenantUpdate :one
UPDATE organization.tenants
SET name = $2
WHERE id = $1
RETURNING id, name, created;
