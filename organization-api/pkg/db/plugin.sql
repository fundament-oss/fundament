-- name: PluginList :many
SELECT id, name, description
FROM zappstore.plugins
WHERE deleted IS NULL
ORDER BY name;

-- name: PluginTagsList :many
SELECT pt.plugin_id, t.id, t.name
FROM zappstore.plugins_tags pt
JOIN zappstore.tags t ON t.id = pt.tag_id
WHERE t.deleted IS NULL
ORDER BY t.name;

-- name: PluginCategoriesList :many
SELECT cp.plugin_id, c.id, c.name
FROM zappstore.categories_plugins cp
JOIN zappstore.categories c ON c.id = cp.tag_id
WHERE c.deleted IS NULL
ORDER BY c.name;
