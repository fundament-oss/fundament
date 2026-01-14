
-- name: ProjectListByOrganizationID :many
-- Note: RLS policies filter to only projects the user has access to
SELECT id, organization_id, name, created, deleted
FROM tenant.projects
WHERE organization_id = $1 AND deleted IS NULL
ORDER BY created DESC;

-- name: ProjectGetByID :one
SELECT id, organization_id, name, created, deleted
FROM tenant.projects
WHERE id = $1 AND deleted IS NULL;

-- name: ProjectCreate :one
INSERT INTO tenant.projects (organization_id, name)
VALUES ($1, $2)
RETURNING id;

-- name: ProjectCreateWithMember :one
-- Creates a project and adds the creator as admin in a single atomic operation
WITH new_project AS (
    INSERT INTO tenant.projects (organization_id, name)
    VALUES ($1, $2)
    RETURNING id
)
INSERT INTO tenant.project_members (project_id, user_id, role)
SELECT id, $3, 'admin' FROM new_project
RETURNING (SELECT id FROM new_project);

-- name: ProjectUpdate :execrows
UPDATE tenant.projects
SET name = COALESCE(sqlc.narg('name'), name)
WHERE id = $1 AND deleted IS NULL;

-- name: ProjectDelete :execrows
UPDATE tenant.projects
SET deleted = NOW()
WHERE id = $1 AND deleted IS NULL;
