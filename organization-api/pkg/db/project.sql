
-- name: ProjectListByOrganizationID :many
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

-- name: ProjectUpdate :execrows
UPDATE tenant.projects
SET name = COALESCE(sqlc.narg('name'), name)
WHERE id = $1 AND deleted IS NULL;

-- name: ProjectDelete :execrows
UPDATE tenant.projects
SET deleted = NOW()
WHERE id = $1 AND deleted IS NULL;
