-- name: MembershipCreateOrAccept :one
-- Assigns a user to an organization in one statement. A new membership is
-- accepted right away: an operator assigning someone is not an invitation to
-- answer. A pending invitation becomes the accepted membership instead, and
-- keeps the permission it was sent with unless one is given. An accepted
-- membership is left alone and returns no row. The trigger puts the row on
-- the authz outbox, so OpenFGA follows.
INSERT INTO tenant.organizations_users (organization_id, user_id, permission, status)
VALUES (@organization_id, @user_id, @permission, 'accepted')
ON CONFLICT (organization_id, user_id) WHERE deleted IS NULL AND status NOT IN ('declined', 'revoked')
DO UPDATE SET
  status = 'accepted',
  permission = CASE
    WHEN @keep_invited_permission::boolean THEN organizations_users.permission
    ELSE EXCLUDED.permission
  END
WHERE organizations_users.status = 'pending'
RETURNING
  id,
  permission,
  (xmax = 0)::boolean AS inserted;

-- name: MembershipList :many
SELECT
  organizations_users.id,
  organizations_users.user_id,
  users.name,
  users.email,
  users.external_ref,
  organizations_users.permission,
  organizations_users.status,
  organizations_users.created
FROM tenant.organizations_users
INNER JOIN tenant.users
  ON users.id = organizations_users.user_id
WHERE organizations_users.organization_id = @organization_id
  AND organizations_users.deleted IS NULL
  AND users.deleted IS NULL
ORDER BY organizations_users.created DESC;

-- name: MembershipDelete :execrows
-- Revokes and soft-deletes a membership or pending invitation, the way the
-- organization-api does it; the update trigger puts the row on the authz
-- outbox so the user's tuples are removed from OpenFGA. A declined invitation
-- is nothing to remove, so it does not count.
UPDATE tenant.organizations_users
SET deleted = now(), status = 'revoked'
WHERE organization_id = @organization_id
  AND user_id = @user_id
  AND status IN ('pending', 'accepted')
  AND deleted IS NULL;

-- name: MembershipRevokeAllForUser :execrows
-- Revokes every live membership and invitation of a user, for when the user
-- row itself is deleted.
UPDATE tenant.organizations_users
SET deleted = now(), status = 'revoked'
WHERE user_id = @user_id
  AND status IN ('pending', 'accepted')
  AND deleted IS NULL;
