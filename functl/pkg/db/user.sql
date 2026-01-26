-- name: UserCreate :one
INSERT INTO tenant.users (
  organization_id,
  name,
  external_id
)
SELECT
  organizations.id,
  @name::text,
  @external_ref::text
FROM tenant.organizations
WHERE organizations.name = @organization_name::text
RETURNING
  id,
  organization_id,
  name,
  external_id,
  created;

-- name: UserList :many
SELECT
  id,
  name,
  external_id,
  created
FROM tenant.users
WHERE organization_id = @organization_id AND deleted IS NULL
ORDER BY created DESC;

-- name: UserDelete :execrows
UPDATE tenant.users
SET deleted = NOW()
FROM tenant.organizations
WHERE
  users.organization_id = organizations.id
  AND organizations.name = @organization_name::text
  AND users.name = @user_name::text
  AND users.deleted IS NULL;
