-- name: PluginList :many
SELECT id, name
FROM tenant.plugins
ORDER BY name;
