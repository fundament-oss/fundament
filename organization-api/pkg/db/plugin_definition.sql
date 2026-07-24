-- name: PluginDefinitionGetActive :one
SELECT appstore.plugin_definitions.id, appstore.plugin_definitions.plugin_id, appstore.plugin_definitions.plugin_version, appstore.plugin_definitions.manifest, appstore.plugin_definitions.hash, appstore.plugin_definitions.created
FROM appstore.plugin_definitions
INNER JOIN appstore.plugins ON appstore.plugins.id = appstore.plugin_definitions.plugin_id
WHERE appstore.plugins.name = $1 AND appstore.plugins.deleted IS NULL AND appstore.plugin_definitions.plugin_version = $2 AND appstore.plugin_definitions.deleted IS NULL;

-- name: PluginDefinitionGetByPluginVersionHash :one
SELECT id, plugin_id, plugin_version, manifest, hash, created
FROM appstore.plugin_definitions
WHERE plugin_id = $1 AND plugin_version = $2 AND hash = $3 AND deleted IS NULL;

-- name: PluginDefinitionInsert :one
INSERT INTO appstore.plugin_definitions (plugin_id, plugin_version, manifest, hash)
VALUES ($1, $2, $3, $4)
RETURNING id, plugin_id, plugin_version, hash, created;

-- name: PluginDefinitionSoftDelete :execrows
UPDATE appstore.plugin_definitions
SET deleted = now()
WHERE appstore.plugin_definitions.plugin_id = $1 AND appstore.plugin_definitions.plugin_version = $2 AND appstore.plugin_definitions.deleted IS NULL;
