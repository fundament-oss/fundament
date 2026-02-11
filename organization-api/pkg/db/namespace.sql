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

-- name: NamespaceGetByClusterAndName :one
SELECT n.id, n.project_id, n.cluster_id, n.name, n.created, n.deleted
FROM tenant.namespaces n
JOIN tenant.clusters c ON c.id = n.cluster_id
WHERE c.name = sqlc.arg('cluster_name') AND n.name = sqlc.arg('namespace_name') AND n.deleted IS NULL AND c.deleted IS NULL;

-- name: NamespaceGetByProjectAndName :one
SELECT n.id, n.project_id, n.cluster_id, n.name, n.created, n.deleted
FROM tenant.namespaces n
JOIN tenant.projects p ON p.id = n.project_id
WHERE p.name = sqlc.arg('project_name') AND n.name = sqlc.arg('namespace_name') AND n.deleted IS NULL AND p.deleted IS NULL;
