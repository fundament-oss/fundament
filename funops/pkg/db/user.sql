-- name: UserCreate :one
-- Registers a user by email ahead of their first sign-in. external_ref stays
-- NULL until the authn-api claims the row when someone signs in at this
-- address, at which point any memberships assigned here are already theirs.
INSERT INTO tenant.users (name, email)
VALUES (@name::text, @email::text)
RETURNING
  id,
  name,
  external_ref,
  email,
  created;

-- name: UserGetByID :one
SELECT
  id,
  name,
  external_ref,
  email,
  created
FROM tenant.users
WHERE id = @id
  AND deleted IS NULL;

-- name: UserFindByEmail :many
-- Matched case-insensitively, the way the authn-api and the invite flow match
-- it. Email is not unique, so more than one match is an ambiguity for the
-- caller to report rather than something to pick from here.
SELECT
  id,
  name,
  external_ref,
  email,
  created
FROM tenant.users
WHERE lower(email) = lower(@email::text)
  AND deleted IS NULL
ORDER BY created;

-- name: UserList :many
-- Every user, with the names of the organizations they belong to or are
-- invited to. Users without any such organization have an empty array.
SELECT
  users.id,
  users.name,
  users.external_ref,
  users.email,
  users.created,
  COALESCE(
    array_agg(organizations.name ORDER BY organizations.name)
      FILTER (WHERE organizations.id IS NOT NULL),
    '{}'
  )::text[] AS organization_names
FROM tenant.users
LEFT JOIN tenant.organizations_users
  ON organizations_users.user_id = users.id
  AND organizations_users.deleted IS NULL
  AND organizations_users.status IN ('pending', 'accepted')
LEFT JOIN tenant.organizations
  ON organizations.id = organizations_users.organization_id
  AND organizations.deleted IS NULL
WHERE users.deleted IS NULL
GROUP BY users.id
ORDER BY users.created DESC;
