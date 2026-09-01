-- name: PresetList :many
SELECT appstore.presets.id, appstore.presets.name, appstore.presets.description
FROM appstore.presets
ORDER BY appstore.presets.name;

-- name: PresetPluginsList :many
-- Membership for every preset at once, as PluginTagsList does for tags.
-- preset_plugins_select_catalog gates each row through the plugin's own policy,
-- which covers RESTRICTED and soft-deleted listings. The published check is the
-- storefront's own rule and lives here for the same reason it does in PluginList:
-- plugins_select_catalog stopped enforcing it so the catalog could serve
-- plugin-controller unpublished manifests.
SELECT appstore.preset_plugins.preset_id, appstore.preset_plugins.plugin_id
FROM appstore.preset_plugins
WHERE EXISTS (
  SELECT 1
  FROM appstore.plugin_definitions
  WHERE appstore.plugin_definitions.plugin_id = appstore.preset_plugins.plugin_id
    AND appstore.plugin_definitions.published IS NOT NULL
);
