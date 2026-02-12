-- name: UserGetByExternalRef :one
SELECT id, name, external_ref, email, created
FROM tenant.users
WHERE external_ref = $1 AND deleted IS NULL;

-- name: UserCreate :one
INSERT INTO tenant.users (name, external_ref, email)
VALUES ($1, $2, $3)
RETURNING id, name, external_ref, email, created;

-- name: UserUpdate :one
UPDATE tenant.users
SET name = $2
WHERE external_ref = $1
RETURNING id, name, external_ref, email, created;

-- name: UserUpsert :one
INSERT INTO tenant.users (name, external_ref, email)
VALUES ($1, $2, $3)
ON CONFLICT (external_ref) WHERE deleted IS NULL
DO UPDATE SET name = EXCLUDED.name, email = EXCLUDED.email
RETURNING id, name, external_ref, email, created;

-- name: UserGetByID :one
SELECT id, name, external_ref, email, created
FROM tenant.users
WHERE id = $1 AND deleted IS NULL;

-- name: UserGetByEmail :one
-- Get a user by email who has no external_ref (pending invitation)
SELECT id, name, external_ref, email, created
FROM tenant.users
WHERE email = $1 AND external_ref IS NULL AND deleted IS NULL
LIMIT 1;

-- name: UserSetExternalRef :exec
UPDATE tenant.users SET external_ref = $2, name = $3 WHERE id = $1;

-- name: OrganizationCreate :one
INSERT INTO tenant.organizations (name)
VALUES ($1)
RETURNING id, name, created;

-- name: OrganizationUserCreate :one
-- Creates a membership for a user in an organization
INSERT INTO tenant.organizations_users (organization_id, user_id, role, status)
VALUES ($1, $2, $3, 'accepted')
RETURNING id, organization_id, user_id, role, status, created;

-- name: UserListOrganizations :many
-- Get the organizations a user belongs to (only accepted memberships)
SELECT
    organizations_users.organization_id,
    organizations_users.role,
    organizations_users.status
FROM tenant.organizations_users
WHERE organizations_users.user_id = $1
    AND organizations_users.status = 'accepted'
    AND organizations_users.deleted IS NULL
ORDER BY organizations_users.created ASC;

-- name: OrganizationUserAccept :exec
-- Transitions a pending invitation to accepted when an invited user logs in
UPDATE tenant.organizations_users
SET status = 'accepted'
WHERE user_id = $1
    AND status = 'pending'
    AND deleted IS NULL;

-- name: APIKeyGetByHash :one
-- Uses SECURITY DEFINER function to bypass RLS (we don't know org_id before lookup)
SELECT id, organization_id, user_id, name, token_prefix, expires, revoked, last_used, created, deleted
FROM authn.api_key_get_by_hash($1);
