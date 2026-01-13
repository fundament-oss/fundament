-- name: NamespaceListByClusterID :many
SELECT id, cluster_id, name, created, deleted
FROM tenant.namespaces
WHERE cluster_id = $1 AND deleted IS NULL
ORDER BY name ASC;

-- name: NamespaceGetByID :one
SELECT id, cluster_id, name, created, deleted
FROM tenant.namespaces
WHERE id = $1 AND deleted IS NULL;
