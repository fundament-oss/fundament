-- name: UserGetByExternalID :one
SELECT id, organization_id, name, external_id, email, created
FROM tenant.users
WHERE external_id = $1 AND deleted IS NULL;

-- name: UserCreate :one
INSERT INTO tenant.users (organization_id, name, external_id, email)
VALUES ($1, $2, $3, $4)
RETURNING id, organization_id, name, external_id, email, created;

-- name: UserUpdate :one
UPDATE tenant.users
SET name = $2
WHERE external_id = $1
RETURNING id, organization_id, name, external_id, email, created;

-- name: UserUpsert :one
INSERT INTO tenant.users (organization_id, name, external_id, email)
VALUES ($1, $2, $3, $4)
ON CONFLICT (external_id, deleted)
DO UPDATE SET name = EXCLUDED.name, email = EXCLUDED.email
RETURNING id, organization_id, name, external_id, email, created;

-- name: UserGetByID :one
SELECT id, organization_id, name, external_id, email, created
FROM tenant.users
WHERE id = $1 AND deleted IS NULL;

-- name: UserGetByEmail :one
SELECT id, organization_id, name, external_id, email, created
FROM tenant.users
WHERE email = $1 AND external_id IS NULL AND deleted IS NULL
LIMIT 1;

-- name: UserSetExternalID :exec
UPDATE tenant.users SET external_id = $2, name = $3 WHERE id = $1;

-- name: OrganizationCreate :one
INSERT INTO tenant.organizations (name)
VALUES ($1)
RETURNING id, name, created;
