-- name: PresetList :many
SELECT id, name, description
FROM appstore.presets
ORDER BY name;

-- name: PresetPluginsList :many
SELECT preset_id, plugin_id
FROM appstore.preset_plugins;
