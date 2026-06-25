
-- name: ProjectList :many
SELECT id, cluster_id, name, alias, created, deleted
FROM tenant.projects
WHERE deleted IS NULL
ORDER BY created DESC;

-- name: ProjectListByClusterID :many
SELECT id, cluster_id, name, alias, created, deleted,
    (SELECT COUNT(*)
     FROM tenant.namespaces
     WHERE namespaces.project_id = projects.id AND namespaces.deleted IS NULL) AS namespace_count,
    (SELECT COUNT(*)
     FROM tenant.project_members
     WHERE project_members.project_id = projects.id AND project_members.deleted IS NULL) AS member_count
FROM tenant.projects
WHERE cluster_id = $1 AND deleted IS NULL
ORDER BY created DESC;

-- name: ProjectGetByID :one
SELECT id, cluster_id, name, alias, created, deleted
FROM tenant.projects
WHERE id = $1 AND deleted IS NULL;

-- name: ProjectGetByName :one
SELECT id, cluster_id, name, alias, created, deleted
FROM tenant.projects
WHERE name = $1 AND deleted IS NULL;

-- name: ProjectCreate :one
INSERT INTO tenant.projects (cluster_id, name, alias)
VALUES ($1, $2, $3)
RETURNING id;

-- name: ProjectUpdate :execrows
UPDATE tenant.projects
SET alias = COALESCE(sqlc.narg('alias'), alias)
WHERE id = $1 AND deleted IS NULL;

-- name: ProjectDelete :execrows
UPDATE tenant.projects
SET deleted = NOW()
WHERE id = $1 AND deleted IS NULL;
