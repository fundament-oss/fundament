-- name: PresetList :many
SELECT id, name, description
FROM zappstore.presets
ORDER BY name;

-- name: PresetPluginsList :many
SELECT preset_id, plugin_id
FROM zappstore.preset_plugins;
