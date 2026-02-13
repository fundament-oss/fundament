
-- name: MemberList :many
SELECT
    users.id,
    organizations_users.organization_id,
    users.name,
    users.external_ref,
    users.email,
    organizations_users.role,
    organizations_users.created
FROM tenant.users
INNER JOIN tenant.organizations_users
    ON organizations_users.user_id = users.id
WHERE organizations_users.deleted IS NULL
    AND users.deleted IS NULL
ORDER BY organizations_users.created DESC;

-- name: MemberGetByEmail :one
SELECT
    users.id,
    organizations_users.organization_id,
    users.name,
    users.external_ref,
    users.email,
    organizations_users.role,
    organizations_users.created
FROM tenant.users
INNER JOIN tenant.organizations_users
    ON organizations_users.user_id = users.id
WHERE users.email = @email::text
    AND organizations_users.deleted IS NULL
    AND users.deleted IS NULL;

-- name: MemberInviteUser :one
-- Creates a new user record for an invited member (no external_ref yet)
INSERT INTO tenant.users (name, email)
VALUES (@email::text, @email::text)
RETURNING id, name, external_ref, email, created;

-- name: MemberInviteMembership :one
-- Creates the organization membership for an invited user
INSERT INTO tenant.organizations_users (organization_id, user_id, role)
VALUES (@organization_id, @user_id, @role::text)
RETURNING id, organization_id, user_id, role, created;

-- name: MemberDelete :exec
-- Soft-delete the organization membership (not the user - they may be in other orgs)
UPDATE tenant.organizations_users
SET deleted = NOW()
WHERE user_id = @user_id
    AND organization_id = @organization_id
    AND deleted IS NULL;
