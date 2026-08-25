-- name: PluginList :many
-- The ILIKE filter is deliberate rather than full-text search: the catalog is
-- small and unpaginated, and FTS would cost a tsvector column plus a trigger.
SELECT
  appstore.plugins.id,
  appstore.plugins.name,
  appstore.plugins.display_name,
  appstore.plugins.description_short,
  appstore.plugins.organization_id,
  appstore.plugins.image,
  appstore.plugins.featured,
  (
    SELECT appstore.plugin_definitions.id
    FROM appstore.plugin_definitions
    WHERE appstore.plugin_definitions.plugin_id = appstore.plugins.id
    ORDER BY appstore.plugin_definitions.published DESC
    LIMIT 1
  ) AS latest_version_id,
  (
    SELECT MIN(appstore.plugin_definitions.published)
    FROM appstore.plugin_definitions
    WHERE appstore.plugin_definitions.plugin_id = appstore.plugins.id
  ) AS published
FROM appstore.plugins
WHERE
  (
    sqlc.narg('query')::text IS NULL
    OR appstore.plugins.name ILIKE '%' || sqlc.narg('query')::text || '%'
    OR appstore.plugins.display_name ILIKE '%' || sqlc.narg('query')::text || '%'
    OR appstore.plugins.description_short ILIKE '%' || sqlc.narg('query')::text || '%'
    OR appstore.plugins.description ILIKE '%' || sqlc.narg('query')::text || '%'
    OR EXISTS (
      SELECT 1
      FROM appstore.plugins_tags
      JOIN appstore.tags ON appstore.tags.id = appstore.plugins_tags.tag_id
      WHERE appstore.plugins_tags.plugin_id = appstore.plugins.id
        AND appstore.tags.deleted IS NULL
        AND appstore.tags.name ILIKE '%' || sqlc.narg('query')::text || '%'
    )
    OR EXISTS (
      SELECT 1
      FROM tenant.organizations
      WHERE tenant.organizations.id = appstore.plugins.organization_id
        AND (
          tenant.organizations.name ILIKE '%' || sqlc.narg('query')::text || '%'
          OR tenant.organizations.alias ILIKE '%' || sqlc.narg('query')::text || '%'
        )
    )
  )
  AND (
    sqlc.narg('category_id')::uuid IS NULL
    OR EXISTS (
      SELECT 1
      FROM appstore.categories_plugins
      WHERE appstore.categories_plugins.plugin_id = appstore.plugins.id
        AND appstore.categories_plugins.category_id = sqlc.narg('category_id')::uuid
    )
  )
  AND (sqlc.arg('featured_only')::bool = false OR appstore.plugins.featured)
ORDER BY
  CASE WHEN sqlc.arg('sort')::text = 'featured' AND appstore.plugins.featured THEN 0 ELSE 1 END,
  CASE WHEN sqlc.arg('sort')::text = 'recently_added' THEN (
    SELECT MIN(appstore.plugin_definitions.published)
    FROM appstore.plugin_definitions
    WHERE appstore.plugin_definitions.plugin_id = appstore.plugins.id
  ) END DESC NULLS LAST,
  COALESCE(NULLIF(appstore.plugins.display_name, ''), appstore.plugins.name) ASC;

-- name: PluginGetByID :one
SELECT
  appstore.plugins.id,
  appstore.plugins.name,
  appstore.plugins.display_name,
  appstore.plugins.description_short,
  appstore.plugins.description,
  appstore.plugins.organization_id,
  appstore.plugins.image,
  appstore.plugins.featured,
  appstore.plugins.author_name,
  appstore.plugins.author_url,
  appstore.plugins.repository_url,
  appstore.plugins.license,
  (
    SELECT appstore.plugin_definitions.id
    FROM appstore.plugin_definitions
    WHERE appstore.plugin_definitions.plugin_id = appstore.plugins.id
    ORDER BY appstore.plugin_definitions.published DESC
    LIMIT 1
  ) AS latest_version_id,
  (
    SELECT MIN(appstore.plugin_definitions.published)
    FROM appstore.plugin_definitions
    WHERE appstore.plugin_definitions.plugin_id = appstore.plugins.id
  ) AS published
FROM appstore.plugins
WHERE appstore.plugins.id = sqlc.arg('id')::uuid;

-- name: PluginLabelsList :many
SELECT appstore.plugin_labels.plugin_id, appstore.plugin_labels.name
FROM appstore.plugin_labels
ORDER BY appstore.plugin_labels.name;

-- name: PluginLabelsListByPluginID :many
SELECT appstore.plugin_labels.plugin_id, appstore.plugin_labels.name
FROM appstore.plugin_labels
WHERE appstore.plugin_labels.plugin_id = sqlc.arg('plugin_id')::uuid
ORDER BY appstore.plugin_labels.name;

-- name: PluginTagsList :many
SELECT appstore.plugins_tags.plugin_id, appstore.tags.name
FROM appstore.plugins_tags
JOIN appstore.tags ON appstore.tags.id = appstore.plugins_tags.tag_id
WHERE appstore.tags.deleted IS NULL
ORDER BY appstore.tags.name;

-- name: PluginTagsListByPluginID :many
SELECT appstore.plugins_tags.plugin_id, appstore.tags.name
FROM appstore.plugins_tags
JOIN appstore.tags ON appstore.tags.id = appstore.plugins_tags.tag_id
WHERE appstore.plugins_tags.plugin_id = sqlc.arg('plugin_id')::uuid AND appstore.tags.deleted IS NULL
ORDER BY appstore.tags.name;

-- name: PluginCategoriesList :many
SELECT appstore.categories_plugins.plugin_id, appstore.categories.id
FROM appstore.categories_plugins
JOIN appstore.categories ON appstore.categories.id = appstore.categories_plugins.category_id
WHERE appstore.categories.deleted IS NULL
ORDER BY appstore.categories.name;

-- name: PluginCategoriesListByPluginID :many
SELECT appstore.categories_plugins.plugin_id, appstore.categories.id
FROM appstore.categories_plugins
JOIN appstore.categories ON appstore.categories.id = appstore.categories_plugins.category_id
WHERE appstore.categories_plugins.plugin_id = sqlc.arg('plugin_id')::uuid AND appstore.categories.deleted IS NULL
ORDER BY appstore.categories.name;

-- name: PluginFeaturesListByPluginID :many
SELECT appstore.plugin_features.title, appstore.plugin_features.body
FROM appstore.plugin_features
WHERE appstore.plugin_features.plugin_id = sqlc.arg('plugin_id')::uuid AND appstore.plugin_features.deleted IS NULL
ORDER BY appstore.plugin_features.position, appstore.plugin_features.title;

-- name: PluginDocumentationLinksList :many
SELECT
  appstore.plugin_documentation_links.id,
  appstore.plugin_documentation_links.title,
  appstore.plugin_documentation_links.url_name,
  appstore.plugin_documentation_links.url
FROM appstore.plugin_documentation_links
WHERE appstore.plugin_documentation_links.plugin_id = sqlc.arg('plugin_id')::uuid
ORDER BY appstore.plugin_documentation_links.title;

-- name: PluginVersionListByPluginID :many
-- Joined through appstore.plugins so the plugin's own RLS policy gates the rows:
-- a restricted or soft-deleted listing must not keep leaking its version history
-- to anyone who already knows the id. The check cannot live in
-- plugin_definitions' policy instead — plugins' policy already references
-- plugin_definitions, and the two would recurse.
SELECT
  appstore.plugin_definitions.id,
  appstore.plugin_definitions.plugin_version,
  appstore.plugin_definitions.published,
  appstore.plugin_definitions.hash,
  appstore.plugin_definitions.release_notes
FROM appstore.plugin_definitions
JOIN appstore.plugins ON appstore.plugins.id = appstore.plugin_definitions.plugin_id
WHERE appstore.plugin_definitions.plugin_id = sqlc.arg('plugin_id')::uuid
ORDER BY appstore.plugin_definitions.published DESC;

-- name: PluginLatestPublishedDefinition :one
SELECT appstore.plugin_definitions.manifest
FROM appstore.plugin_definitions
WHERE appstore.plugin_definitions.plugin_id = sqlc.arg('plugin_id')::uuid
ORDER BY appstore.plugin_definitions.published DESC
LIMIT 1;
