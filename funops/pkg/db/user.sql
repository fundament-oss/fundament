-- name: UserCreate :one
INSERT INTO tenant.users (
  name,
  external_ref
)
VALUES (@name::text, @external_ref::text)
RETURNING
  id,
  name,
  external_ref,
  created;

-- name: UserCreateMembership :one
-- Creates a membership for a user in an organization (by organization name)
INSERT INTO tenant.organizations_users (
  organization_id,
  user_id,
  role,
  status
)
SELECT
    organizations.id,
    @user_id,
    @role::text,
    'accepted'
FROM tenant.organizations
WHERE organizations.name = @organization_name::text
RETURNING
  id,
  organization_id,
  user_id,
  role,
  status,
  created;

-- name: UserList :many
SELECT
    users.id,
    users.name,
    users.external_ref,
    users.created
FROM tenant.users
INNER JOIN tenant.organizations_users
    ON organizations_users.user_id = users.id
WHERE organizations_users.organization_id = @organization_id
    AND organizations_users.deleted IS NULL
    AND users.deleted IS NULL
ORDER BY users.created DESC;

-- name: UserDelete :execrows
UPDATE tenant.organizations_users
SET deleted = NOW()
FROM tenant.organizations, tenant.users
WHERE
    organizations_users.organization_id = organizations.id
    AND organizations_users.user_id = users.id
    AND organizations.name = @organization_name::text
    AND users.name = @user_name::text
    AND organizations_users.deleted IS NULL;
