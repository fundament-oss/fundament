
-- name: MemberList :many
SELECT
    organizations_users.id,
    organizations_users.user_id,
    organizations_users.organization_id,
    organizations_users.user_id,
    users.name,
    users.external_ref,
    users.email,
    organizations_users.permission,
    organizations_users.status,
    organizations_users.created
FROM tenant.users
INNER JOIN tenant.organizations_users
    ON organizations_users.user_id = users.id
WHERE organizations_users.deleted IS NULL
    AND users.deleted IS NULL
ORDER BY organizations_users.created DESC;

-- name: MemberGetByID :one
SELECT
    organizations_users.id,
    organizations_users.organization_id,
    organizations_users.user_id,
    users.name,
    users.external_ref,
    users.email,
    organizations_users.permission,
    organizations_users.status,
    organizations_users.created
FROM tenant.users
INNER JOIN tenant.organizations_users
    ON organizations_users.user_id = users.id
WHERE organizations_users.id = @id
    AND organizations_users.deleted IS NULL
    AND users.deleted IS NULL;

-- name: MemberGetByUserID :one
SELECT
    organizations_users.id,
    organizations_users.organization_id,
    organizations_users.user_id,
    users.name,
    users.external_ref,
    users.email,
    organizations_users.permission,
    organizations_users.status,
    organizations_users.created
FROM tenant.users
INNER JOIN tenant.organizations_users
    ON organizations_users.user_id = users.id
WHERE organizations_users.user_id = @user_id
    AND organizations_users.deleted IS NULL
    AND users.deleted IS NULL;

-- name: MemberUpdatePermission :execrows
UPDATE tenant.organizations_users
SET permission = $2
WHERE
    id = $1
    AND organization_id = $3
    AND deleted IS NULL;

-- name: MemberDelete :exec
UPDATE tenant.organizations_users
SET deleted = NOW(), status = 'revoked'
WHERE id = @id
  AND deleted IS NULL;
