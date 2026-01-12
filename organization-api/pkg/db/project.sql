
-- name: ProjectListByOrganizationID :many
SELECT id, organization_id, name, created
FROM tenant.projects
WHERE organization_id = $1
ORDER BY created DESC;

-- name: ProjectGetByID :one
SELECT id, organization_id, name, created
FROM tenant.projects
WHERE id = $1;

-- name: ProjectCreate :one
INSERT INTO tenant.projects (organization_id, name)
VALUES ($1, $2)
RETURNING id;

-- name: ProjectUpdate :execrows
UPDATE tenant.projects
SET name = COALESCE(sqlc.narg('name'), name)
WHERE id = $1;

-- name: ProjectDelete :execrows
DELETE FROM tenant.projects
WHERE id = $1;
