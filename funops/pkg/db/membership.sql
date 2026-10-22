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
  (old.id IS NULL)::boolean AS inserted;

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

-- name: MembershipLockOrganizationsForRemoval :exec
-- Locks the organizations whose membership or invitation of the user the
-- removal revokes, or only the given one, the way DeleteMember in
-- organization-api locks its organization, so removals in one organization
-- take turns: the protect_last_admin trigger
-- locks each project in the order the user's project memberships are
-- scanned, and two removals that each hold one project and wait for the other
-- would deadlock. Taken before the membership rows, and in id order across
-- organizations, so two removals never wait for each other's organization.
SELECT tenant.organizations.id
FROM tenant.organizations
WHERE tenant.organizations.id IN (
    SELECT tenant.organizations_users.organization_id
    FROM tenant.organizations_users
    WHERE tenant.organizations_users.user_id = @user_id
      AND tenant.organizations_users.status IN ('pending', 'accepted')
      AND tenant.organizations_users.deleted IS NULL
      AND (sqlc.narg('organization_id')::uuid IS NULL
           OR tenant.organizations_users.organization_id = sqlc.narg('organization_id')::uuid)
)
ORDER BY tenant.organizations.id
FOR NO KEY UPDATE;

-- name: MembershipDelete :execrows
-- Revokes and soft-deletes a membership or pending invitation, the way the
-- organization-api does it; the update trigger puts the row on the authz
-- outbox so the user's tuples are removed from OpenFGA, and the trigger
-- remove_project_memberships removes the user from the organization's
-- projects (refused while they are the only admin of one). A declined
-- invitation is nothing to remove, so it does not count.
UPDATE tenant.organizations_users
SET deleted = now(), status = 'revoked'
WHERE organization_id = @organization_id
  AND user_id = @user_id
  AND status IN ('pending', 'accepted')
  AND deleted IS NULL;

-- name: MembershipRevokeAllForUser :execrows
-- Revokes every live membership and invitation of a user, for when the user
-- row itself is deleted. As with MembershipDelete, the user leaves the
-- organizations' projects too.
UPDATE tenant.organizations_users
SET deleted = now(), status = 'revoked'
WHERE user_id = @user_id
  AND status IN ('pending', 'accepted')
  AND deleted IS NULL;

-- name: MembershipRevokeAllForOrganization :execrows
-- Revokes every live membership and invitation in an organization, for when
-- the organization itself is deleted. The update trigger puts each row on the
-- authz outbox, so the members' tuples are removed from OpenFGA.
UPDATE tenant.organizations_users
SET deleted = now(), status = 'revoked'
WHERE organization_id = @organization_id
  AND status IN ('pending', 'accepted')
  AND deleted IS NULL;

-- name: ProjectListWhereUserIsLastAdmin :many
-- Projects whose only admin is the given user, in one organization or, without
-- one, in all of them: what keeps MembershipDelete or MembershipRevokeAllForUser
-- from removing the user. Project names are unique only within a cluster.
SELECT
  tenant.organizations.name AS organization_name,
  tenant.clusters.name AS cluster_name,
  tenant.projects.name AS project_name
FROM tenant.projects
JOIN tenant.clusters ON tenant.clusters.id = tenant.projects.cluster_id
JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
JOIN tenant.project_members ON tenant.project_members.project_id = tenant.projects.id
WHERE (sqlc.narg('organization_id')::uuid IS NULL OR tenant.clusters.organization_id = sqlc.narg('organization_id')::uuid)
  AND tenant.projects.deleted IS NULL
  AND tenant.project_members.user_id = @user_id
  AND tenant.project_members.role = 'admin'
  AND tenant.project_members.deleted IS NULL
  -- Inside this subquery tenant.project_members names the subquery's own rows,
  -- the other members of the project, not the outer user's membership.
  AND NOT EXISTS (
    SELECT 1
    FROM tenant.project_members
    WHERE tenant.project_members.project_id = tenant.projects.id
      AND tenant.project_members.role = 'admin'
      AND tenant.project_members.deleted IS NULL
      AND tenant.project_members.user_id != @user_id
  )
ORDER BY tenant.organizations.name, tenant.clusters.name, tenant.projects.name;
