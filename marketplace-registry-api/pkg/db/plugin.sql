-- Queries for registry.v1.PublicationService. Every one of them runs under
-- fun_marketplace_registry_api, whose policies scope rows to the caller's
-- organization, so none of them filters on organization_id itself.

-- name: PluginList :many
-- latest_published_version_id is derived rather than stored: the newest version
-- carrying a published timestamp is the one the listing went live with.
SELECT
	appstore.plugins.id,
	appstore.plugins.name,
	appstore.plugins.display_name,
	appstore.plugins.description_short,
	appstore.plugins.description,
	appstore.plugins.organization_id,
	appstore.plugins.image,
	appstore.plugins.author_name,
	appstore.plugins.author_url,
	appstore.plugins.repository_url,
	appstore.plugins.license,
	appstore.plugins.visibility,
	appstore.plugins.created,
	appstore.plugins.updated,
	(SELECT appstore.plugin_definitions.id
	   FROM appstore.plugin_definitions
	  WHERE appstore.plugin_definitions.plugin_id = appstore.plugins.id
	    AND appstore.plugin_definitions.published IS NOT NULL
	    AND appstore.plugin_definitions.deleted IS NULL
	  ORDER BY appstore.plugin_definitions.published DESC
	  LIMIT 1) AS latest_published_version_id
FROM appstore.plugins
WHERE appstore.plugins.deleted IS NULL
ORDER BY appstore.plugins.name ASC;

-- name: PluginGetByID :one
SELECT
	appstore.plugins.id,
	appstore.plugins.name,
	appstore.plugins.display_name,
	appstore.plugins.description_short,
	appstore.plugins.description,
	appstore.plugins.organization_id,
	appstore.plugins.image,
	appstore.plugins.author_name,
	appstore.plugins.author_url,
	appstore.plugins.repository_url,
	appstore.plugins.license,
	appstore.plugins.visibility,
	appstore.plugins.created,
	appstore.plugins.updated,
	(SELECT appstore.plugin_definitions.id
	   FROM appstore.plugin_definitions
	  WHERE appstore.plugin_definitions.plugin_id = appstore.plugins.id
	    AND appstore.plugin_definitions.published IS NOT NULL
	    AND appstore.plugin_definitions.deleted IS NULL
	  ORDER BY appstore.plugin_definitions.published DESC
	  LIMIT 1) AS latest_published_version_id
FROM appstore.plugins
WHERE appstore.plugins.id = sqlc.arg('id')::uuid
  AND appstore.plugins.deleted IS NULL;

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
RETURNING appstore.plugins.id;

-- name: PluginUpdate :exec
-- name is absent: it is immutable once reserved (FUN-5).
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
WHERE appstore.plugins.id = sqlc.arg('id')::uuid
  AND appstore.plugins.deleted IS NULL;

-- name: PluginSoftDelete :exec
-- The deleted guard makes this idempotent and, together with the one on
-- PluginUpdate, keeps a delete landing between a handler's lookup and
-- its write from being undone: the two run on separate pooled connections.
UPDATE appstore.plugins SET deleted = now()
WHERE appstore.plugins.id = sqlc.arg('id')::uuid
  AND appstore.plugins.deleted IS NULL;

-- name: PluginTagsListByPluginID :many
SELECT appstore.tags.name
FROM appstore.plugins_tags
JOIN appstore.tags ON appstore.tags.id = appstore.plugins_tags.tag_id
WHERE appstore.plugins_tags.plugin_id = sqlc.arg('plugin_id')::uuid
  AND appstore.tags.deleted IS NULL
ORDER BY appstore.tags.name ASC;

-- name: TagInsertIfMissing :exec
-- Tags have no identity a client holds, so they are addressed by name and
-- created on demand. DO NOTHING rather than DO UPDATE deliberately: tags are a
-- vocabulary shared across every publisher, and DO UPDATE would need UPDATE on
-- the table, letting one publisher rename another's tag.
INSERT INTO appstore.tags (name) VALUES (sqlc.arg('name')::text)
ON CONFLICT (name, deleted) DO NOTHING;

-- name: TagGetByName :one
SELECT appstore.tags.id
FROM appstore.tags
WHERE appstore.tags.name = sqlc.arg('name')::text
  AND appstore.tags.deleted IS NULL;

-- name: PluginTagInsert :exec
INSERT INTO appstore.plugins_tags (plugin_id, tag_id)
VALUES (sqlc.arg('plugin_id')::uuid, sqlc.arg('tag_id')::uuid)
ON CONFLICT DO NOTHING;

-- name: PluginTagsDeleteByPluginID :exec
-- An edge, not an entity: UpdatePlugin's full replacement removes it outright
-- rather than soft-deleting.
DELETE FROM appstore.plugins_tags
WHERE appstore.plugins_tags.plugin_id = sqlc.arg('plugin_id')::uuid;

-- name: PluginCategoriesListByPluginID :many
SELECT appstore.categories_plugins.category_id
FROM appstore.categories_plugins
WHERE appstore.categories_plugins.plugin_id = sqlc.arg('plugin_id')::uuid
ORDER BY appstore.categories_plugins.category_id ASC;

-- name: PluginCategoryInsert :exec
INSERT INTO appstore.categories_plugins (plugin_id, category_id)
VALUES (sqlc.arg('plugin_id')::uuid, sqlc.arg('category_id')::uuid)
ON CONFLICT DO NOTHING;

-- name: PluginCategoriesDeleteByPluginID :exec
DELETE FROM appstore.categories_plugins
WHERE appstore.categories_plugins.plugin_id = sqlc.arg('plugin_id')::uuid;

-- name: PluginAllowedOrgsListByPluginID :many
SELECT appstore.plugin_allowed_organizations.organization_id
FROM appstore.plugin_allowed_organizations
WHERE appstore.plugin_allowed_organizations.plugin_id = sqlc.arg('plugin_id')::uuid
ORDER BY appstore.plugin_allowed_organizations.organization_id ASC;

-- name: PluginAllowedOrgInsert :exec
INSERT INTO appstore.plugin_allowed_organizations (plugin_id, organization_id)
VALUES (sqlc.arg('plugin_id')::uuid, sqlc.arg('organization_id')::uuid)
ON CONFLICT DO NOTHING;

-- name: PluginAllowedOrgsDeleteByPluginID :exec
DELETE FROM appstore.plugin_allowed_organizations
WHERE appstore.plugin_allowed_organizations.plugin_id = sqlc.arg('plugin_id')::uuid;

-- name: PluginDocLinksListByPluginID :many
SELECT
	appstore.plugin_documentation_links.id,
	appstore.plugin_documentation_links.title,
	appstore.plugin_documentation_links.url_name,
	appstore.plugin_documentation_links.url
FROM appstore.plugin_documentation_links
WHERE appstore.plugin_documentation_links.plugin_id = sqlc.arg('plugin_id')::uuid
  AND appstore.plugin_documentation_links.deleted IS NULL
ORDER BY appstore.plugin_documentation_links.position ASC;

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
-- cannot be pulled into this one; the row count is how the handler tells an
-- unknown id from a successful update.
UPDATE appstore.plugin_documentation_links SET
	title = sqlc.arg('title')::text,
	url_name = sqlc.arg('url_name')::text,
	url = sqlc.arg('url')::text,
	position = sqlc.arg('position')::integer
WHERE appstore.plugin_documentation_links.id = sqlc.arg('id')::uuid
  AND appstore.plugin_documentation_links.plugin_id = sqlc.arg('plugin_id')::uuid
  AND appstore.plugin_documentation_links.deleted IS NULL;

-- name: PluginDocLinksSoftDeleteNotIn :exec
-- A documentation link has an id the API hands out, so replacement soft-deletes
-- rather than removing. The links the request still carries are excluded: their
-- ids survive the update, which is what makes an id worth handing out.
UPDATE appstore.plugin_documentation_links SET deleted = now()
WHERE appstore.plugin_documentation_links.plugin_id = sqlc.arg('plugin_id')::uuid
  AND appstore.plugin_documentation_links.deleted IS NULL
  AND appstore.plugin_documentation_links.id <> ALL(sqlc.arg('kept_ids')::uuid[]);

-- name: PluginFeaturesListByPluginID :many
SELECT
	appstore.plugin_features.id,
	appstore.plugin_features.title,
	appstore.plugin_features.body
FROM appstore.plugin_features
WHERE appstore.plugin_features.plugin_id = sqlc.arg('plugin_id')::uuid
  AND appstore.plugin_features.deleted IS NULL
ORDER BY appstore.plugin_features.position ASC;

-- name: PluginFeatureInsert :exec
INSERT INTO appstore.plugin_features (plugin_id, title, body, position)
VALUES (
	sqlc.arg('plugin_id')::uuid,
	sqlc.arg('title')::text,
	sqlc.arg('body')::text,
	sqlc.arg('position')::integer
);

-- name: PluginFeatureUpdate :execrows
-- Mirrors PluginDocLinkUpdate: scoped by plugin_id as well as id, with
-- the row count standing in for "was that id real".
UPDATE appstore.plugin_features SET
	title = sqlc.arg('title')::text,
	body = sqlc.arg('body')::text,
	position = sqlc.arg('position')::integer
WHERE appstore.plugin_features.id = sqlc.arg('id')::uuid
  AND appstore.plugin_features.plugin_id = sqlc.arg('plugin_id')::uuid
  AND appstore.plugin_features.deleted IS NULL;

-- name: PluginFeaturesSoftDeleteNotIn :exec
-- A feature block has an id the API hands out, so replacement soft-deletes
-- rather than removing, and spares the ids the request carried back.
UPDATE appstore.plugin_features SET deleted = now()
WHERE appstore.plugin_features.plugin_id = sqlc.arg('plugin_id')::uuid
  AND appstore.plugin_features.deleted IS NULL
  AND appstore.plugin_features.id <> ALL(sqlc.arg('kept_ids')::uuid[]);
