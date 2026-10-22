-- name: ProjectMemberCreate :one
INSERT INTO tenant.project_members (project_id, user_id, role)
VALUES ($1, $2, $3)
RETURNING id;

-- name: ProjectMemberList :many
SELECT
    project_members.id,
    project_members.project_id,
    project_members.user_id,
    project_members.role,
    project_members.created,
    users.name as user_name,
    users.external_ref as user_external_ref
FROM tenant.project_members
INNER JOIN tenant.users
  ON users.id = project_members.user_id
WHERE project_members.project_id = $1
  AND project_members.deleted IS NULL
ORDER BY project_members.created ASC;

-- name: ProjectMemberGetByID :one
SELECT
    project_members.id,
    project_members.project_id,
    project_members.user_id,
    project_members.role,
    project_members.created,
    users.name as user_name
FROM tenant.project_members
INNER JOIN tenant.users
  ON users.id = project_members.user_id
WHERE project_members.id = $1
  AND project_members.deleted IS NULL;

-- name: ProjectMemberUpdateRole :execrows
UPDATE tenant.project_members
SET role = $2
WHERE id = $1
AND deleted IS NULL;

-- name: ProjectMemberDelete :execrows
UPDATE tenant.project_members
SET deleted = now()
WHERE id = $1
AND deleted IS NULL;

-- name: ProjectListWhereUserIsLastAdmin :many
-- Projects of the current organization whose only admin is the given user.
SELECT tenant.projects.name
FROM tenant.projects
JOIN tenant.clusters ON tenant.clusters.id = tenant.projects.cluster_id
JOIN tenant.project_members ON tenant.project_members.project_id = tenant.projects.id
WHERE tenant.clusters.organization_id = authn.current_organization_id()
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
ORDER BY tenant.projects.name;
