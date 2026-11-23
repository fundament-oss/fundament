-- Queries run when a version is approved (common/pluginlisting.Sync), inside
-- the approving transaction of marketplace-admin-api or funops.

-- name: VersionGetManifest :one
SELECT plugin_id, plugin_version, manifest
FROM appstore.plugin_definitions
WHERE id = sqlc.arg('id')::uuid
  AND deleted IS NULL;

-- name: PluginLock :one
-- Serializes concurrent approvals of one plugin, so each sees the other's
-- published version before deciding which is newest.
SELECT id
FROM appstore.plugins
WHERE id = sqlc.arg('id')::uuid
  AND deleted IS NULL
FOR UPDATE;

-- name: VersionsPublishedExcept :many
SELECT plugin_version
FROM appstore.plugin_definitions
WHERE plugin_id = sqlc.arg('plugin_id')::uuid
  AND id <> sqlc.arg('id')::uuid
  AND published IS NOT NULL
  AND deleted IS NULL;

-- name: PluginSyncDefinitionFields :exec
UPDATE appstore.plugins SET
	display_name = sqlc.arg('display_name')::text,
	description_short = sqlc.arg('description_short')::text,
	repository_url = sqlc.narg('repository_url')::text,
	license = sqlc.arg('license')::text,
	updated = now()
WHERE id = sqlc.arg('id')::uuid
  AND deleted IS NULL;

-- name: PluginTagsDelete :exec
-- An edge, not an entity: replacement removes it outright.
DELETE FROM appstore.plugins_tags
WHERE plugin_id = sqlc.arg('plugin_id')::uuid;

-- name: TagInsertIfMissing :exec
-- DO NOTHING, not DO UPDATE: tags are a vocabulary shared across publishers.
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

-- name: SubmissionOpenDraft :one
-- The approval runs before the round is closed, so the round is still open.
SELECT listing_draft
FROM appstore.submissions
WHERE plugin_definition_id = sqlc.arg('plugin_definition_id')::uuid
  AND closed IS NULL
  AND deleted IS NULL;

-- name: PluginApplyDraftContent :exec
UPDATE appstore.plugins SET
	description = sqlc.arg('description')::text,
	image = sqlc.arg('image')::text,
	author_name = sqlc.narg('author_name')::text,
	author_url = sqlc.narg('author_url')::text,
	logo = sqlc.arg('logo')::text,
	logo_dark = sqlc.arg('logo_dark')::text,
	updated = now()
WHERE id = sqlc.arg('id')::uuid
  AND deleted IS NULL;

-- name: PluginDraftClear :exec
-- Only when the pending draft is still the one applied: edits made after the
-- submission stay pending for the next round.
UPDATE appstore.plugins SET listing_draft = NULL
WHERE id = sqlc.arg('id')::uuid
  AND listing_draft = sqlc.arg('applied')::jsonb;

-- name: PluginCategoriesDelete :exec
DELETE FROM appstore.categories_plugins
WHERE plugin_id = sqlc.arg('plugin_id')::uuid;

-- name: PluginCategoryInsert :exec
INSERT INTO appstore.categories_plugins (plugin_id, category_id)
VALUES (sqlc.arg('plugin_id')::uuid, sqlc.arg('category_id')::uuid)
ON CONFLICT DO NOTHING;

-- name: PluginFeaturesSoftDeleteNotIn :exec
-- Features and links have ids the API hands out, so replacement soft-deletes
-- the ones the draft dropped and keeps the ids it carried.
UPDATE appstore.plugin_features SET deleted = now()
WHERE plugin_id = sqlc.arg('plugin_id')::uuid
  AND deleted IS NULL
  AND id <> ALL(sqlc.arg('kept_ids')::uuid[]);

-- name: PluginFeatureInsert :exec
INSERT INTO appstore.plugin_features (plugin_id, title, body, position)
VALUES (
	sqlc.arg('plugin_id')::uuid,
	sqlc.arg('title')::text,
	sqlc.arg('body')::text,
	sqlc.arg('position')::integer
);

-- name: PluginFeatureUpdate :execrows
-- Scoped by plugin_id too, so a draft cannot pull in another listing's row.
UPDATE appstore.plugin_features SET
	title = sqlc.arg('title')::text,
	body = sqlc.arg('body')::text,
	position = sqlc.arg('position')::integer
WHERE id = sqlc.arg('id')::uuid
  AND plugin_id = sqlc.arg('plugin_id')::uuid
  AND deleted IS NULL;

-- name: PluginDocLinksSoftDeleteNotIn :exec
UPDATE appstore.plugin_documentation_links SET deleted = now()
WHERE plugin_id = sqlc.arg('plugin_id')::uuid
  AND deleted IS NULL
  AND id <> ALL(sqlc.arg('kept_ids')::uuid[]);

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
UPDATE appstore.plugin_documentation_links SET
	title = sqlc.arg('title')::text,
	url_name = sqlc.arg('url_name')::text,
	url = sqlc.arg('url')::text,
	position = sqlc.arg('position')::integer
WHERE id = sqlc.arg('id')::uuid
  AND plugin_id = sqlc.arg('plugin_id')::uuid
  AND deleted IS NULL;
