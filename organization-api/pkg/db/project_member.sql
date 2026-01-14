-- name: ProjectMemberCreate :one
INSERT INTO tenant.project_members (project_id, user_id, role)
VALUES ($1, $2, $3)
RETURNING id;

-- name: ProjectMemberList :many
SELECT
    pm.id,
    pm.project_id,
    pm.user_id,
    pm.role,
    pm.created,
    u.name as user_name,
    u.external_id as user_external_id
FROM tenant.project_members pm
JOIN tenant.users u ON u.id = pm.user_id
WHERE pm.project_id = $1
ORDER BY pm.created ASC;

-- name: ProjectMemberGetByID :one
SELECT id, project_id, user_id, role, created
FROM tenant.project_members
WHERE id = $1;

-- name: ProjectMemberGetByProjectAndUser :one
SELECT id, project_id, user_id, role, created
FROM tenant.project_members
WHERE project_id = $1 AND user_id = $2;

-- name: ProjectMemberUpdateRole :execrows
UPDATE tenant.project_members
SET role = $2
WHERE id = $1;

-- name: ProjectMemberDelete :execrows
DELETE FROM tenant.project_members
WHERE id = $1;

-- name: ProjectMemberCountAdmins :one
SELECT COUNT(*)::int as admin_count
FROM tenant.project_members
WHERE project_id = $1 AND role = 'admin';
