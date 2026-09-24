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
-- Every user, with the names of the organizations they are an accepted member
-- of and, separately, the ones they hold an unanswered invitation to. An
-- invitation is not membership: someone invited to one organization and in
-- none is still waiting for an operator.
SELECT
  users.id,
  users.name,
  users.external_ref,
  users.email,
  users.created,
  COALESCE(
    array_agg(organizations.name ORDER BY organizations.name)
      FILTER (WHERE organizations.id IS NOT NULL AND organizations_users.status = 'accepted'),
    '{}'
  )::text[] AS organization_names,
  COALESCE(
    array_agg(organizations.name ORDER BY organizations.name)
      FILTER (WHERE organizations.id IS NOT NULL AND organizations_users.status = 'pending'),
    '{}'
  )::text[] AS invitation_names
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

-- name: UserDeleteRegistration :execrows
-- Soft-deletes a user nobody has signed in with: a registration made at a
-- wrong address would otherwise stay claimable by whoever signs in there. An
-- account somebody has signed in to is not deleted here.
UPDATE tenant.users
SET deleted = now()
WHERE id = @id
  AND external_ref IS NULL
  AND deleted IS NULL;
