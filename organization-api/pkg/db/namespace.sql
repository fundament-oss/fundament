-- name: NamespaceListByClusterID :many
SELECT id, project_id, cluster_id, name, created, deleted
FROM tenant.namespaces
WHERE cluster_id = $1 AND deleted IS NULL
ORDER BY name ASC;

-- name: NamespaceGetByID :one
SELECT id, project_id, cluster_id, name, created, deleted
FROM tenant.namespaces
WHERE id = $1 AND deleted IS NULL;

-- name: NamespaceCreate :one
INSERT INTO tenant.namespaces (project_id, cluster_id, name)
VALUES ($1, $2, $3)
RETURNING id;

-- name: NamespaceDelete :execrows
UPDATE tenant.namespaces
SET deleted = NOW()
WHERE id = $1 AND deleted IS NULL;

-- name: NamespaceListByProjectID :many
SELECT id, project_id, cluster_id, name, created, deleted
FROM tenant.namespaces
WHERE project_id = $1 AND deleted IS NULL
ORDER BY name ASC;
