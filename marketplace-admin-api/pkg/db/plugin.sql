-- Read queries backing the ids a Submission hands out (FUN-20): the catalog
-- carries only approved public listings and the registry authorizes ownership,
-- so the backoffice serves its own reads.

-- name: PluginList :many
SELECT
	id,
	name,
	display_name,
	description_short,
	description,
	organization_id,
	image,
	created,
	updated
FROM appstore.plugins
WHERE deleted IS NULL
ORDER BY name ASC;

-- name: PluginGetByID :one
SELECT
	id,
	name,
	display_name,
	description_short,
	description,
	organization_id,
	image,
	created,
	updated
FROM appstore.plugins
WHERE id = sqlc.arg('id')::uuid
  AND deleted IS NULL;

-- name: PluginTagsListByPluginIDs :many
-- Takes an array so ListPlugins reads every listing's tags in one query rather
-- than one per listing.
SELECT plugins_tags.plugin_id, tags.name
FROM appstore.plugins_tags
JOIN appstore.tags ON tags.id = plugins_tags.tag_id
WHERE plugins_tags.plugin_id = ANY(sqlc.arg('plugin_ids')::uuid[])
  AND tags.deleted IS NULL
ORDER BY plugins_tags.plugin_id ASC, tags.name ASC;

-- name: PluginCategoriesListByPluginIDs :many
-- Joined to the vocabulary so a category soft-deleted after it was attached
-- stops being handed out: ListCategories would never resolve the id.
SELECT categories_plugins.plugin_id, categories_plugins.category_id
FROM appstore.categories_plugins
JOIN appstore.categories ON categories.id = categories_plugins.category_id
WHERE categories_plugins.plugin_id = ANY(sqlc.arg('plugin_ids')::uuid[])
  AND categories.deleted IS NULL
ORDER BY categories_plugins.plugin_id ASC, categories_plugins.category_id ASC;

-- name: PluginVersionGetByID :one
-- Selects the manifest: capabilities and permissions — what the plugin will be
-- allowed to do once installed — are derived from the pinned bytes rather than
-- stored in columns, so they can never drift from the hash the install pins
-- against. submitted is the latest round's, matching the registry's read.
SELECT
	plugin_definitions.id,
	plugin_definitions.plugin_id,
	plugin_definitions.plugin_version,
	plugin_definitions.image,
	plugin_definitions.hash,
	plugin_definitions.status,
	plugin_definitions.release_notes,
	plugin_definitions.manifest,
	plugin_definitions.created,
	plugin_definitions.published,
	(SELECT submissions.submitted
	   FROM appstore.submissions
	  WHERE submissions.plugin_definition_id = plugin_definitions.id
	    AND submissions.deleted IS NULL
	  ORDER BY submissions.submitted DESC
	  LIMIT 1) AS submitted
FROM appstore.plugin_definitions
JOIN appstore.plugins ON plugins.id = plugin_definitions.plugin_id
WHERE plugin_definitions.id = sqlc.arg('id')::uuid
  AND plugin_definitions.deleted IS NULL
  AND plugins.deleted IS NULL;

-- name: CategoryList :many
SELECT id, name
FROM appstore.categories
WHERE deleted IS NULL
ORDER BY name ASC;

-- name: PublisherList :many
-- The RLS policy already limits this to organizations with a listing; deleted
-- stays in the query because lifecycle filtering is the queries' job (FUN-20).
SELECT tenant.organizations.id, tenant.organizations.name, tenant.organizations.alias
FROM tenant.organizations
WHERE tenant.organizations.deleted IS NULL
ORDER BY tenant.organizations.alias, tenant.organizations.name;
