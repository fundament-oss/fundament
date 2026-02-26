-- name: InviteCreateMembership :one
-- Creates the organization membership for an invited user
INSERT INTO tenant.organizations_users (organization_id, user_id, permission, status)
VALUES (@organization_id, @user_id, @permission::text, 'pending')
RETURNING id, organization_id, user_id, permission, status, created;

-- name: InviteList :many
-- Lists pending invitations for the current user across all organizations
SELECT
    organizations_users.id,
    organizations_users.organization_id,
    organizations.display_name,
    organizations_users.permission,
    organizations_users.status,
    organizations_users.created
FROM tenant.organizations_users
INNER JOIN tenant.organizations
    ON organizations.id = organizations_users.organization_id
WHERE organizations_users.user_id = @user_id
    AND organizations_users.status = 'pending'
    AND organizations_users.deleted IS NULL
    AND organizations.deleted IS NULL
ORDER BY organizations_users.created DESC;

-- name: InviteAccept :execrows
-- User accepts a pending invitation to an organization
UPDATE tenant.organizations_users
SET status = 'accepted'
WHERE id = @id
    AND status = 'pending'
    AND deleted IS NULL;

-- name: InviteDecline :execrows
-- User declines a pending invitation to an organization
UPDATE tenant.organizations_users
SET status = 'declined'
WHERE id = @id
    AND status = 'pending'
    AND deleted IS NULL;
