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
    users.external_id as user_external_id
FROM tenant.project_members
INNER JOIN tenant.users
  ON users.id = project_members.user_id
WHERE project_members.project_id = $1
  AND project_members.deleted IS NULL
ORDER BY project_members.created ASC;

-- name: ProjectMemberUpdateRole :execrows
UPDATE tenant.project_members
SET role = $2
WHERE id = $1
AND deleted IS NULL;

-- name: ProjectMemberGetByID :one
SELECT id, project_id, user_id, role
FROM tenant.project_members
WHERE id = $1
AND deleted IS NULL;

-- name: ProjectMemberDelete :execrows
UPDATE tenant.project_members
SET deleted = now()
WHERE id = $1
AND deleted IS NULL;
