-- name: PluginList :many
SELECT appstore.plugins.id, appstore.plugins.name, appstore.plugins.display_name, appstore.plugins.description_short, appstore.plugins.description, appstore.plugins.image,
  COALESCE((
    SELECT appstore.plugin_definitions.plugin_version
    FROM appstore.plugin_definitions
    WHERE appstore.plugin_definitions.plugin_id = appstore.plugins.id AND appstore.plugin_definitions.deleted IS NULL
    ORDER BY appstore.plugin_definitions.created DESC
    LIMIT 1
  ), '')::text AS latest_version,
  COALESCE((
    SELECT appstore.plugin_definitions.hash
    FROM appstore.plugin_definitions
    WHERE appstore.plugin_definitions.plugin_id = appstore.plugins.id AND appstore.plugin_definitions.deleted IS NULL
    ORDER BY appstore.plugin_definitions.created DESC
    LIMIT 1
  ), '')::text AS latest_hash
FROM appstore.plugins
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

-- name: PluginGetByID :one
SELECT appstore.plugins.id, appstore.plugins.name, appstore.plugins.display_name, appstore.plugins.description_short, appstore.plugins.description, appstore.plugins.image, appstore.plugins.author_name, appstore.plugins.author_url, appstore.plugins.repository_url,
  COALESCE((
    SELECT appstore.plugin_definitions.plugin_version
    FROM appstore.plugin_definitions
    WHERE appstore.plugin_definitions.plugin_id = appstore.plugins.id AND appstore.plugin_definitions.deleted IS NULL
    ORDER BY appstore.plugin_definitions.created DESC
    LIMIT 1
  ), '')::text AS latest_version,
  COALESCE((
    SELECT appstore.plugin_definitions.hash
    FROM appstore.plugin_definitions
    WHERE appstore.plugin_definitions.plugin_id = appstore.plugins.id AND appstore.plugin_definitions.deleted IS NULL
    ORDER BY appstore.plugin_definitions.created DESC
    LIMIT 1
  ), '')::text AS latest_hash
FROM appstore.plugins
WHERE appstore.plugins.id = $1 AND appstore.plugins.deleted IS NULL;

-- name: PluginTagsListByPluginID :many
SELECT pt.plugin_id, t.id, t.name
FROM appstore.plugins_tags pt
JOIN appstore.tags t ON t.id = pt.tag_id
WHERE pt.plugin_id = $1 AND t.deleted IS NULL
ORDER BY t.name;

-- name: PluginCategoriesListByPluginID :many
SELECT cp.plugin_id, c.id, c.name
FROM appstore.categories_plugins cp
JOIN appstore.categories c ON c.id = cp.category_id
WHERE cp.plugin_id = $1 AND c.deleted IS NULL
ORDER BY c.name;

-- name: PluginDocumentationLinksList :many
SELECT id, plugin_id, title, url_name, url
FROM appstore.plugin_documentation_links
WHERE plugin_id = $1
ORDER BY title;
