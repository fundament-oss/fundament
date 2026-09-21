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
-- Get a user by email who has no external_ref (pending invitation).
-- The address is matched case-insensitively: the spelling someone was invited
-- at (or registered at by an operator) and the spelling their identity provider
-- reports are the same address, and missing each other here would sign the
-- invited person in as a stranger without any of their memberships.
SELECT id, name, external_ref, email, created
FROM tenant.users
WHERE lower(email) = lower(@email::text) AND external_ref IS NULL AND deleted IS NULL
ORDER BY created
LIMIT 1;

-- name: UserSetExternalRef :exec
UPDATE tenant.users SET external_ref = $2, name = $3 WHERE id = $1;

-- name: UserListOrganizations :many
-- Get the organizations a user belongs to (only accepted memberships)
SELECT
    organizations_users.organization_id,
    organizations_users.permission,
    organizations_users.status
FROM tenant.organizations_users
WHERE organizations_users.user_id = $1
    AND organizations_users.status = 'accepted'
    AND organizations_users.deleted IS NULL
ORDER BY organizations_users.created ASC;

-- name: APIKeyGetByHash :one
-- Uses SECURITY DEFINER function to bypass RLS (we don't know org_id before lookup)
SELECT id, organization_id, user_id, name, token_prefix, expires, revoked, last_used, created, deleted
FROM authn.api_key_get_by_hash($1);

-- name: APIKeyUpdateLastUsed :exec
-- Uses SECURITY DEFINER function to bypass RLS
SELECT authn.api_key_update_last_used($1);

-- name: ClusterGetByID :one
-- The organization a shoot workload belongs to comes from this row, never
-- from the shoot (FUN-22). Deleted clusters refuse the exchange.
SELECT id, organization_id
FROM tenant.clusters
WHERE id = $1 AND deleted IS NULL;
