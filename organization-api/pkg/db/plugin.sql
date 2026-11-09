-- name: PluginList :many
SELECT appstore.plugins.id, appstore.plugins.name, appstore.plugins.display_name, appstore.plugins.description_short, appstore.plugins.description, appstore.plugins.image,
  tenant.organizations.name AS organization_name,
  COALESCE((
    SELECT appstore.plugin_definitions.plugin_version
    FROM appstore.plugin_definitions
    WHERE appstore.plugin_definitions.plugin_id = appstore.plugins.id AND appstore.plugin_definitions.deleted IS NULL
      AND appstore.plugin_definitions.published IS NOT NULL
    ORDER BY appstore.plugin_definitions.published DESC, appstore.plugin_definitions.id DESC
    LIMIT 1
  ), '')::text AS latest_version,
  COALESCE((
    SELECT appstore.plugin_definitions.hash
    FROM appstore.plugin_definitions
    WHERE appstore.plugin_definitions.plugin_id = appstore.plugins.id AND appstore.plugin_definitions.deleted IS NULL
      AND appstore.plugin_definitions.published IS NOT NULL
    ORDER BY appstore.plugin_definitions.published DESC, appstore.plugin_definitions.id DESC
    LIMIT 1
  ), '')::text AS latest_hash,
  COALESCE((
    SELECT appstore.plugin_definitions.logo
    FROM appstore.plugin_definitions
    WHERE appstore.plugin_definitions.plugin_id = appstore.plugins.id AND appstore.plugin_definitions.deleted IS NULL AND appstore.plugin_definitions.published IS NOT NULL
    ORDER BY appstore.plugin_definitions.published DESC
    LIMIT 1
  ), '')::text AS logo,
  COALESCE((
    SELECT appstore.plugin_definitions.logo_dark
    FROM appstore.plugin_definitions
    WHERE appstore.plugin_definitions.plugin_id = appstore.plugins.id AND appstore.plugin_definitions.deleted IS NULL AND appstore.plugin_definitions.published IS NOT NULL
    ORDER BY appstore.plugin_definitions.published DESC
    LIMIT 1
  ), '')::text AS logo_dark
FROM appstore.plugins
INNER JOIN tenant.organizations ON tenant.organizations.id = appstore.plugins.organization_id
WHERE appstore.plugins.deleted IS NULL
ORDER BY COALESCE(NULLIF(appstore.plugins.display_name, ''), appstore.plugins.name);

-- name: PluginTagsList :many
SELECT pt.plugin_id, t.id, t.name
FROM appstore.plugins_tags pt
JOIN appstore.tags t ON t.id = pt.tag_id
WHERE t.deleted IS NULL
ORDER BY t.name;

-- name: PluginCategoriesList :many
SELECT cp.plugin_id, c.id, c.name
FROM appstore.categories_plugins cp
JOIN appstore.categories c ON c.id = cp.category_id
WHERE c.deleted IS NULL
ORDER BY c.name;
