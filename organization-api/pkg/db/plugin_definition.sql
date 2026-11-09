-- name: PluginDefinitionGetActive :one
SELECT appstore.plugin_definitions.id, appstore.plugin_definitions.plugin_id, appstore.plugin_definitions.plugin_version, appstore.plugin_definitions.manifest, appstore.plugin_definitions.hash, appstore.plugin_definitions.created, appstore.plugin_definitions.logo, appstore.plugin_definitions.menu_icons
FROM appstore.plugin_definitions
INNER JOIN appstore.plugins ON appstore.plugins.id = appstore.plugin_definitions.plugin_id
INNER JOIN tenant.organizations ON tenant.organizations.id = appstore.plugins.organization_id
WHERE tenant.organizations.name = sqlc.arg('organization_name') AND tenant.organizations.deleted IS NULL
  AND appstore.plugins.name = sqlc.arg('plugin_name') AND appstore.plugins.deleted IS NULL
  AND appstore.plugin_definitions.plugin_version = sqlc.arg('plugin_version') AND appstore.plugin_definitions.deleted IS NULL;
