
-- name: PluginListByClusterID :many
SELECT id, cluster_id, plugin_id, created, deleted
FROM tenant.plugins
WHERE cluster_id = $1 AND deleted IS NULL
ORDER BY created DESC;

-- name: PluginGetByID :one
SELECT id, cluster_id, plugin_id, created, deleted
FROM tenant.plugins
WHERE id = $1 AND deleted IS NULL;

-- name: PluginCreate :one
INSERT INTO tenant.plugins (cluster_id, plugin_id)
VALUES ($1, $2)
RETURNING id, cluster_id, plugin_id, created, deleted;

-- name: PluginDelete :exec
UPDATE tenant.plugins
SET deleted = NOW()
WHERE id = $1 AND deleted IS NULL;
