-- name: UserGetByExternalID :one
SELECT id, tenant_id, name, external_id, created
FROM organization.users
WHERE external_id = $1;

-- name: UserCreate :one
INSERT INTO organization.users (tenant_id, name, external_id)
VALUES ($1, $2, $3)
RETURNING id, tenant_id, name, external_id, created;

-- name: UserUpdate :one
UPDATE organization.users
SET name = $2
WHERE external_id = $1
RETURNING id, tenant_id, name, external_id, created;

-- name: UserUpsert :one
INSERT INTO organization.users (tenant_id, name, external_id)
VALUES ($1, $2, $3)
ON CONFLICT (external_id)
DO UPDATE SET name = EXCLUDED.name
RETURNING id, tenant_id, name, external_id, created;

-- name: UserGetByID :one
SELECT id, tenant_id, name, external_id, created
FROM organization.users
WHERE id = $1;

-- name: TenantCreate :one
INSERT INTO organization.tenants (name)
VALUES ($1)
RETURNING id, name, created;
