-- name: ProjectMemberCreate :one
-- Only members of the project's organization (invited or accepted, as the
-- console offers them) can join a project; for anyone else no row is
-- inserted and the query returns no rows. cluster-worker provisions the
-- member on clusters once the invitation is accepted. The membership row is
-- share-locked so a concurrent removal of the member (MemberDelete) is
-- waited for and seen, instead of racing this insert.
INSERT INTO tenant.project_members (project_id, user_id, role)
SELECT @project_id, @user_id, @role
WHERE EXISTS (
    SELECT 1
    FROM tenant.organizations_users
    WHERE tenant.organizations_users.user_id = @user_id
        AND tenant.organizations_users.status IN ('pending', 'accepted')
        AND tenant.organizations_users.deleted IS NULL
        AND EXISTS (
            SELECT 1
            FROM tenant.users
            WHERE tenant.users.id = @user_id
                AND tenant.users.deleted IS NULL
        )
        AND tenant.organizations_users.organization_id = (
            SELECT tenant.clusters.organization_id
            FROM tenant.projects
            JOIN tenant.clusters ON tenant.clusters.id = tenant.projects.cluster_id
            WHERE tenant.projects.id = @project_id
                AND tenant.projects.deleted IS NULL
        )
    FOR SHARE
)
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
