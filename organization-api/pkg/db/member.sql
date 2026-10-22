
-- name: MemberList :many
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
WHERE organizations_users.organization_id = authn.current_organization_id()
    AND organizations_users.deleted IS NULL
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
    AND organizations_users.organization_id = authn.current_organization_id()
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
    AND organizations_users.organization_id = authn.current_organization_id()
    AND organizations_users.deleted IS NULL
    AND users.deleted IS NULL;

-- name: MemberLockAcceptedByUserID :one
-- Locks the user's accepted membership of the current organization against
-- removal until the transaction ends. Revoking the membership removes the
-- user's project memberships (trigger remove_project_memberships), so a
-- project member added under this lock is either refused here or removed by
-- that cleanup.
SELECT tenant.organizations_users.id
FROM tenant.organizations_users
WHERE tenant.organizations_users.user_id = @user_id
    AND tenant.organizations_users.organization_id = authn.current_organization_id()
    AND tenant.organizations_users.status = 'accepted'
    AND tenant.organizations_users.deleted IS NULL
    AND EXISTS (
        SELECT 1
        FROM tenant.users
        WHERE tenant.users.id = tenant.organizations_users.user_id
            AND tenant.users.deleted IS NULL
    )
FOR SHARE;

-- name: MemberUpdatePermission :execrows
UPDATE tenant.organizations_users
SET permission = $2
WHERE
    id = $1
    AND deleted IS NULL;

-- name: MemberDelete :execrows
-- Revokes the membership. The trigger remove_project_memberships removes the
-- user from the organization's projects with it.
UPDATE tenant.organizations_users
SET deleted = NOW(), status = 'revoked'
WHERE id = @id
  AND deleted IS NULL;
