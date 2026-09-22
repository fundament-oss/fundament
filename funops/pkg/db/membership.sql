-- name: MembershipCreate :one
-- Assigns a user to an organization. The membership is accepted right away:
-- an operator assigning someone is not an invitation to answer. The insert
-- trigger puts the row on the authz outbox, so OpenFGA follows.
INSERT INTO tenant.organizations_users (organization_id, user_id, permission, status)
VALUES (@organization_id, @user_id, @permission, 'accepted')
RETURNING
  id,
  organization_id,
  user_id,
  permission,
  status,
  created;

-- name: MembershipAcceptPending :execrows
-- Turns an invitation the user has not answered yet into an accepted
-- membership, for when an operator assigns someone who was already invited.
UPDATE tenant.organizations_users
SET status = 'accepted', permission = @permission
WHERE organization_id = @organization_id
  AND user_id = @user_id
  AND status = 'pending'
  AND deleted IS NULL;

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
-- Revokes and soft-deletes the membership, the way the organization-api does
-- it; the update trigger puts the row on the authz outbox so the user's tuples
-- are removed from OpenFGA.
UPDATE tenant.organizations_users
SET deleted = now(), status = 'revoked'
WHERE organization_id = @organization_id
  AND user_id = @user_id
  AND deleted IS NULL;
