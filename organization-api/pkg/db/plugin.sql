-- name: PluginList :many
SELECT id, name, description
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
JOIN appstore.categories c ON c.id = cp.tag_id
WHERE c.deleted IS NULL
ORDER BY c.name;
