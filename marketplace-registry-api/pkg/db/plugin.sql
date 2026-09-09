-- Queries for registry.v1.PublicationService. Every one of them runs under
-- fun_marketplace_registry_api, whose policies scope rows to the caller's
-- organization, so none of them filters on organization_id itself.

-- name: PluginList :many
-- latest_published_version_id is derived rather than stored: the newest version
-- carrying a published timestamp is the one the listing went live with.
SELECT
	id,
	name,
	display_name,
	description_short,
	description,
	organization_id,
	image,
	author_name,
	author_url,
	repository_url,
	license,
	visibility,
	created,
	updated,
	(SELECT plugin_definitions.id
	   FROM appstore.plugin_definitions
	  WHERE plugin_definitions.plugin_id = plugins.id
	    AND plugin_definitions.published IS NOT NULL
	    AND plugin_definitions.deleted IS NULL
	  ORDER BY plugin_definitions.published DESC
	  LIMIT 1) AS latest_published_version_id
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
	author_name,
	author_url,
	repository_url,
	license,
	visibility,
	created,
	updated,
	(SELECT plugin_definitions.id
	   FROM appstore.plugin_definitions
	  WHERE plugin_definitions.plugin_id = plugins.id
	    AND plugin_definitions.published IS NOT NULL
	    AND plugin_definitions.deleted IS NULL
	  ORDER BY plugin_definitions.published DESC
	  LIMIT 1) AS latest_published_version_id
FROM appstore.plugins
WHERE id = sqlc.arg('id')::uuid
  AND deleted IS NULL;

-- name: PluginCreate :one
-- organization_id is supplied rather than defaulted so the INSERT's WITH CHECK
-- has something to compare; the policy rejects any value but the caller's own.
INSERT INTO appstore.plugins (
	organization_id, name, display_name, description_short, description,
	image, repository_url, license, visibility
) VALUES (
	sqlc.arg('organization_id')::uuid,
	sqlc.arg('name')::text,
	sqlc.arg('display_name')::text,
	sqlc.arg('description_short')::text,
	sqlc.arg('description')::text,
	sqlc.arg('image')::text,
	sqlc.narg('repository_url')::text,
	sqlc.arg('license')::text,
	sqlc.arg('visibility')::text
)
RETURNING id;

-- name: PluginUpdate :execrows
-- name is absent: it is immutable once reserved (FUN-5). Zero rows means the
-- listing was soft-deleted after the handler looked it up.
UPDATE appstore.plugins SET
	display_name = sqlc.arg('display_name')::text,
	description_short = sqlc.arg('description_short')::text,
	description = sqlc.arg('description')::text,
	image = sqlc.arg('image')::text,
	repository_url = sqlc.narg('repository_url')::text,
	author_name = sqlc.narg('author_name')::text,
	author_url = sqlc.narg('author_url')::text,
	license = sqlc.arg('license')::text,
	visibility = sqlc.arg('visibility')::text,
	updated = now()
WHERE id = sqlc.arg('id')::uuid
  AND deleted IS NULL;

-- name: PluginSoftDelete :exec
UPDATE appstore.plugins SET deleted = now()
WHERE id = sqlc.arg('id')::uuid
  AND deleted IS NULL;

-- name: PluginTagsListByPluginIDs :many
-- Takes an array so ListPlugins reads every listing's tags in one query rather
-- than one per listing, as do the other child-row lists here.
SELECT plugins_tags.plugin_id, tags.name
FROM appstore.plugins_tags
JOIN appstore.tags ON tags.id = plugins_tags.tag_id
WHERE plugins_tags.plugin_id = ANY(sqlc.arg('plugin_ids')::uuid[])
  AND tags.deleted IS NULL
ORDER BY plugins_tags.plugin_id ASC, tags.name ASC;

-- name: TagInsertIfMissing :exec
-- DO NOTHING rather than DO UPDATE: tags are a vocabulary shared across every
-- publisher, and DO UPDATE would need UPDATE on the table, letting one publisher
-- rename another's tag.
INSERT INTO appstore.tags (name) VALUES (sqlc.arg('name')::text)
ON CONFLICT (name, deleted) DO NOTHING;

-- name: TagGetByName :one
SELECT id
FROM appstore.tags
WHERE name = sqlc.arg('name')::text
  AND deleted IS NULL;

-- name: PluginTagInsert :exec
INSERT INTO appstore.plugins_tags (plugin_id, tag_id)
VALUES (sqlc.arg('plugin_id')::uuid, sqlc.arg('tag_id')::uuid)
ON CONFLICT DO NOTHING;

-- name: PluginTagsDeleteByPluginID :exec
-- An edge, not an entity: replacement removes it outright rather than
-- soft-deleting.
DELETE FROM appstore.plugins_tags
WHERE plugin_id = sqlc.arg('plugin_id')::uuid;

-- name: PluginCategoriesListByPluginIDs :many
-- Joined to the vocabulary so a category soft-deleted after it was attached
-- stops being handed out: ListCategories would never resolve the id.
SELECT categories_plugins.plugin_id, categories_plugins.category_id
FROM appstore.categories_plugins
JOIN appstore.categories ON categories.id = categories_plugins.category_id
WHERE categories_plugins.plugin_id = ANY(sqlc.arg('plugin_ids')::uuid[])
  AND categories.deleted IS NULL
ORDER BY categories_plugins.plugin_id ASC, categories_plugins.category_id ASC;

-- name: PluginCategoryInsert :exec
INSERT INTO appstore.categories_plugins (plugin_id, category_id)
VALUES (sqlc.arg('plugin_id')::uuid, sqlc.arg('category_id')::uuid)
ON CONFLICT DO NOTHING;

-- name: PluginCategoriesDeleteByPluginID :exec
DELETE FROM appstore.categories_plugins
WHERE plugin_id = sqlc.arg('plugin_id')::uuid;

-- name: PluginAllowedOrgsListByPluginIDs :many
SELECT plugin_id, organization_id
FROM appstore.plugin_allowed_organizations
WHERE plugin_id = ANY(sqlc.arg('plugin_ids')::uuid[])
ORDER BY plugin_id ASC, organization_id ASC;

-- name: PluginAllowedOrgInsert :exec
INSERT INTO appstore.plugin_allowed_organizations (plugin_id, organization_id)
VALUES (sqlc.arg('plugin_id')::uuid, sqlc.arg('organization_id')::uuid)
ON CONFLICT DO NOTHING;

-- name: PluginAllowedOrgsDeleteByPluginID :exec
DELETE FROM appstore.plugin_allowed_organizations
WHERE plugin_id = sqlc.arg('plugin_id')::uuid;

-- name: PluginDocLinksListByPluginIDs :many
SELECT plugin_id, id, title, url_name, url
FROM appstore.plugin_documentation_links
WHERE plugin_id = ANY(sqlc.arg('plugin_ids')::uuid[])
  AND deleted IS NULL
ORDER BY plugin_id ASC, position ASC, id ASC;

-- name: PluginDocLinkInsert :exec
INSERT INTO appstore.plugin_documentation_links (plugin_id, title, url_name, url, position)
VALUES (
	sqlc.arg('plugin_id')::uuid,
	sqlc.arg('title')::text,
	sqlc.arg('url_name')::text,
	sqlc.arg('url')::text,
	sqlc.arg('position')::integer
);

-- name: PluginDocLinkUpdate :execrows
-- Scoped by plugin_id as well as id so a link belonging to another listing
-- cannot be pulled into this one; zero rows means the id was not real.
UPDATE appstore.plugin_documentation_links SET
	title = sqlc.arg('title')::text,
	url_name = sqlc.arg('url_name')::text,
	url = sqlc.arg('url')::text,
	position = sqlc.arg('position')::integer
WHERE id = sqlc.arg('id')::uuid
  AND plugin_id = sqlc.arg('plugin_id')::uuid
  AND deleted IS NULL;

-- name: PluginDocLinksSoftDeleteNotIn :exec
-- A documentation link has an id the API hands out, so replacement soft-deletes
-- rather than removing and spares the ids the request carried back.
UPDATE appstore.plugin_documentation_links SET deleted = now()
WHERE plugin_id = sqlc.arg('plugin_id')::uuid
  AND deleted IS NULL
  AND id <> ALL(sqlc.arg('kept_ids')::uuid[]);

-- name: PluginFeaturesListByPluginIDs :many
SELECT plugin_id, id, title, body
FROM appstore.plugin_features
WHERE plugin_id = ANY(sqlc.arg('plugin_ids')::uuid[])
  AND deleted IS NULL
ORDER BY plugin_id ASC, position ASC, id ASC;

-- name: PluginFeatureInsert :exec
INSERT INTO appstore.plugin_features (plugin_id, title, body, position)
VALUES (
	sqlc.arg('plugin_id')::uuid,
	sqlc.arg('title')::text,
	sqlc.arg('body')::text,
	sqlc.arg('position')::integer
);

-- name: PluginFeatureUpdate :execrows
-- Mirrors PluginDocLinkUpdate.
UPDATE appstore.plugin_features SET
	title = sqlc.arg('title')::text,
	body = sqlc.arg('body')::text,
	position = sqlc.arg('position')::integer
WHERE id = sqlc.arg('id')::uuid
  AND plugin_id = sqlc.arg('plugin_id')::uuid
  AND deleted IS NULL;

-- name: PluginFeaturesSoftDeleteNotIn :exec
-- Mirrors PluginDocLinksSoftDeleteNotIn.
UPDATE appstore.plugin_features SET deleted = now()
WHERE plugin_id = sqlc.arg('plugin_id')::uuid
  AND deleted IS NULL
  AND id <> ALL(sqlc.arg('kept_ids')::uuid[]);
