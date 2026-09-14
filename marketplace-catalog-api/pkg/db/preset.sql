-- name: PresetList :many
SELECT id, name, description
FROM appstore.presets
ORDER BY name;

-- name: PresetPluginsList :many
-- Membership for every preset at once, as PluginTagsList does for tags.
-- preset_plugins_select_catalog gates each row through the plugin's own policy,
-- which covers RESTRICTED and soft-deleted listings. The published check is the
-- storefront's own rule and lives here for the same reason it does in PluginList:
-- plugins_select_catalog stopped enforcing it so the catalog could serve
-- plugin-controller unpublished manifests.
SELECT preset_id, plugin_id
FROM appstore.preset_plugins
WHERE EXISTS (
  SELECT 1
  FROM appstore.plugin_definitions
  WHERE appstore.plugin_definitions.plugin_id = appstore.preset_plugins.plugin_id
    AND appstore.plugin_definitions.published IS NOT NULL
);
