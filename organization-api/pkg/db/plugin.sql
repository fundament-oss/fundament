-- name: PluginList :many
SELECT id, name, description_short, description
FROM appstore.plugins
WHERE deleted IS NULL
ORDER BY name;

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
SELECT id, name, description_short, description, author_name, author_url, repository_url
FROM appstore.plugins
WHERE id = $1 AND deleted IS NULL;

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
