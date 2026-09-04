-- Queries for registry.v1.PublicationService. Every one of them runs under
-- fun_marketplace_registry_api, whose policies scope rows to the caller's
-- organization, so none of them filters on organization_id itself. Names are
-- prefixed Registry because catalog.v1's queries share this generated package.

-- name: RegistryPluginList :many
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

-- name: RegistryPluginGetByID :one
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

-- name: RegistryPluginCreate :one
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

-- name: RegistryPluginUpdate :exec
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
WHERE appstore.plugins.id = sqlc.arg('id')::uuid;

-- name: RegistryPluginSoftDelete :exec
UPDATE appstore.plugins SET deleted = now()
WHERE appstore.plugins.id = sqlc.arg('id')::uuid;

-- name: RegistryPluginTagsListByPluginID :many
SELECT appstore.tags.name
FROM appstore.plugins_tags
JOIN appstore.tags ON appstore.tags.id = appstore.plugins_tags.tag_id
WHERE appstore.plugins_tags.plugin_id = sqlc.arg('plugin_id')::uuid
  AND appstore.tags.deleted IS NULL
ORDER BY appstore.tags.name ASC;

-- name: RegistryTagInsertIfMissing :exec
-- Tags have no identity a client holds, so they are addressed by name and
-- created on demand. DO NOTHING rather than DO UPDATE deliberately: tags are a
-- vocabulary shared across every publisher, and DO UPDATE would need UPDATE on
-- the table, letting one publisher rename another's tag.
INSERT INTO appstore.tags (name) VALUES (sqlc.arg('name')::text)
ON CONFLICT (name, deleted) DO NOTHING;

-- name: RegistryTagGetByName :one
SELECT appstore.tags.id
FROM appstore.tags
WHERE appstore.tags.name = sqlc.arg('name')::text
  AND appstore.tags.deleted IS NULL;

-- name: RegistryPluginTagInsert :exec
INSERT INTO appstore.plugins_tags (plugin_id, tag_id)
VALUES (sqlc.arg('plugin_id')::uuid, sqlc.arg('tag_id')::uuid)
ON CONFLICT DO NOTHING;

-- name: RegistryPluginTagsDeleteByPluginID :exec
-- An edge, not an entity: UpdatePlugin's full replacement removes it outright
-- rather than soft-deleting.
DELETE FROM appstore.plugins_tags
WHERE appstore.plugins_tags.plugin_id = sqlc.arg('plugin_id')::uuid;

-- name: RegistryPluginCategoriesListByPluginID :many
SELECT appstore.categories_plugins.category_id
FROM appstore.categories_plugins
WHERE appstore.categories_plugins.plugin_id = sqlc.arg('plugin_id')::uuid
ORDER BY appstore.categories_plugins.category_id ASC;

-- name: RegistryPluginCategoryInsert :exec
INSERT INTO appstore.categories_plugins (plugin_id, category_id)
VALUES (sqlc.arg('plugin_id')::uuid, sqlc.arg('category_id')::uuid)
ON CONFLICT DO NOTHING;

-- name: RegistryPluginCategoriesDeleteByPluginID :exec
DELETE FROM appstore.categories_plugins
WHERE appstore.categories_plugins.plugin_id = sqlc.arg('plugin_id')::uuid;

-- name: RegistryPluginAllowedOrgsListByPluginID :many
SELECT appstore.plugin_allowed_organizations.organization_id
FROM appstore.plugin_allowed_organizations
WHERE appstore.plugin_allowed_organizations.plugin_id = sqlc.arg('plugin_id')::uuid
ORDER BY appstore.plugin_allowed_organizations.organization_id ASC;

-- name: RegistryPluginAllowedOrgInsert :exec
INSERT INTO appstore.plugin_allowed_organizations (plugin_id, organization_id)
VALUES (sqlc.arg('plugin_id')::uuid, sqlc.arg('organization_id')::uuid)
ON CONFLICT DO NOTHING;

-- name: RegistryPluginAllowedOrgsDeleteByPluginID :exec
DELETE FROM appstore.plugin_allowed_organizations
WHERE appstore.plugin_allowed_organizations.plugin_id = sqlc.arg('plugin_id')::uuid;

-- name: RegistryPluginDocLinksListByPluginID :many
SELECT
	appstore.plugin_documentation_links.id,
	appstore.plugin_documentation_links.title,
	appstore.plugin_documentation_links.url_name,
	appstore.plugin_documentation_links.url
FROM appstore.plugin_documentation_links
WHERE appstore.plugin_documentation_links.plugin_id = sqlc.arg('plugin_id')::uuid
  AND appstore.plugin_documentation_links.deleted IS NULL
ORDER BY appstore.plugin_documentation_links.position ASC;

-- name: RegistryPluginDocLinkInsert :exec
INSERT INTO appstore.plugin_documentation_links (plugin_id, title, url_name, url, position)
VALUES (
	sqlc.arg('plugin_id')::uuid,
	sqlc.arg('title')::text,
	sqlc.arg('url_name')::text,
	sqlc.arg('url')::text,
	sqlc.arg('position')::integer
);

-- name: RegistryPluginDocLinksSoftDeleteByPluginID :exec
-- A documentation link has an id the API hands out, so replacement soft-deletes.
UPDATE appstore.plugin_documentation_links SET deleted = now()
WHERE appstore.plugin_documentation_links.plugin_id = sqlc.arg('plugin_id')::uuid
  AND appstore.plugin_documentation_links.deleted IS NULL;

-- name: RegistryPluginFeaturesListByPluginID :many
SELECT
	appstore.plugin_features.title,
	appstore.plugin_features.body
FROM appstore.plugin_features
WHERE appstore.plugin_features.plugin_id = sqlc.arg('plugin_id')::uuid
  AND appstore.plugin_features.deleted IS NULL
ORDER BY appstore.plugin_features.position ASC;

-- name: RegistryPluginFeatureInsert :exec
INSERT INTO appstore.plugin_features (plugin_id, title, body, position)
VALUES (
	sqlc.arg('plugin_id')::uuid,
	sqlc.arg('title')::text,
	sqlc.arg('body')::text,
	sqlc.arg('position')::integer
);

-- name: RegistryPluginFeaturesSoftDeleteByPluginID :exec
UPDATE appstore.plugin_features SET deleted = now()
WHERE appstore.plugin_features.plugin_id = sqlc.arg('plugin_id')::uuid
  AND appstore.plugin_features.deleted IS NULL;
