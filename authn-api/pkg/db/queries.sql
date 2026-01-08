-- name: UserGetByExternalID :one
SELECT id, organization_id, name, external_id, created
FROM tenant.users
WHERE external_id = $1;

-- name: UserCreate :one
INSERT INTO tenant.users (organization_id, name, external_id)
VALUES ($1, $2, $3)
RETURNING id, organization_id, name, external_id, created;

-- name: UserUpdate :one
UPDATE tenant.users
SET name = $2
WHERE external_id = $1
RETURNING id, organization_id, name, external_id, created;

-- name: UserUpsert :one
INSERT INTO tenant.users (organization_id, name, external_id)
VALUES ($1, $2, $3)
ON CONFLICT (external_id)
DO UPDATE SET name = EXCLUDED.name
RETURNING id, organization_id, name, external_id, created;

-- name: UserGetByID :one
SELECT id, organization_id, name, external_id, created
FROM tenant.users
WHERE id = $1;

-- name: OrganizationCreate :one
INSERT INTO tenant.organizations (name)
VALUES ($1)
RETURNING id, name, created;
