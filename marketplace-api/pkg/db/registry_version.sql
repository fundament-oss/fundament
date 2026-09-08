-- Version and submission queries for registry.v1.PublicationService.
--
-- Every version query joins through appstore.plugins even where plugin_id alone
-- would find the row. plugin_definitions' own policy gates on ownership through
-- that join already, but writing it explicitly keeps a query that is later
-- changed to select by version id from silently widening its scope.

-- name: RegistryPluginVersionListByPluginID :many
SELECT
	appstore.plugin_definitions.id,
	appstore.plugin_definitions.plugin_id,
	appstore.plugin_definitions.plugin_version,
	appstore.plugin_definitions.manifest,
	appstore.plugin_definitions.hash,
	appstore.plugin_definitions.status,
	appstore.plugin_definitions.release_notes,
	appstore.plugin_definitions.created,
	appstore.plugin_definitions.published
FROM appstore.plugin_definitions
JOIN appstore.plugins ON appstore.plugins.id = appstore.plugin_definitions.plugin_id
WHERE appstore.plugin_definitions.plugin_id = sqlc.arg('plugin_id')::uuid
  AND appstore.plugin_definitions.deleted IS NULL
  AND appstore.plugins.deleted IS NULL
ORDER BY appstore.plugin_definitions.created DESC;

-- name: RegistryPluginVersionGetByID :one
SELECT
	appstore.plugin_definitions.id,
	appstore.plugin_definitions.plugin_id,
	appstore.plugin_definitions.plugin_version,
	appstore.plugin_definitions.manifest,
	appstore.plugin_definitions.hash,
	appstore.plugin_definitions.status,
	appstore.plugin_definitions.release_notes,
	appstore.plugin_definitions.created,
	appstore.plugin_definitions.published
FROM appstore.plugin_definitions
JOIN appstore.plugins ON appstore.plugins.id = appstore.plugin_definitions.plugin_id
WHERE appstore.plugin_definitions.id = sqlc.arg('id')::uuid
  AND appstore.plugin_definitions.deleted IS NULL
  AND appstore.plugins.deleted IS NULL;

-- name: RegistryPluginVersionCreate :one
-- Lands in DRAFT. The hash is computed by the server from the manifest bytes,
-- never supplied by the client.
INSERT INTO appstore.plugin_definitions (
	plugin_id, plugin_version, manifest, hash, release_notes, status
) VALUES (
	sqlc.arg('plugin_id')::uuid,
	sqlc.arg('plugin_version')::text,
	sqlc.arg('manifest')::bytea,
	sqlc.arg('hash')::text,
	sqlc.arg('release_notes')::text,
	'draft'
)
RETURNING appstore.plugin_definitions.id, appstore.plugin_definitions.created;

-- name: RegistryPluginVersionSetStatus :exec
-- No join to plugins: an UPDATE cannot reference it without a FROM clause, and
-- the policy already gates ownership. The caller has read the version through
-- RegistryPluginVersionGetByID, which does check the plugin is not deleted.
UPDATE appstore.plugin_definitions SET status = sqlc.arg('status')::text
WHERE appstore.plugin_definitions.id = sqlc.arg('id')::uuid
  AND appstore.plugin_definitions.deleted IS NULL;

-- name: RegistryPluginVersionsSoftDeleteByPluginID :exec
-- DeletePlugin cascades: a soft-deleted listing must not strand active versions.
UPDATE appstore.plugin_definitions SET deleted = now()
WHERE appstore.plugin_definitions.plugin_id = sqlc.arg('plugin_id')::uuid
  AND appstore.plugin_definitions.deleted IS NULL;

-- name: RegistrySubmissionCreate :one
INSERT INTO appstore.submissions (plugin_definition_id, submitter_user_id)
VALUES (sqlc.arg('plugin_definition_id')::uuid, sqlc.arg('submitter_user_id')::uuid)
RETURNING appstore.submissions.id;

-- name: RegistrySubmissionOpenByDefinitionID :one
SELECT appstore.submissions.id, appstore.submissions.submitted
FROM appstore.submissions
WHERE appstore.submissions.plugin_definition_id = sqlc.arg('plugin_definition_id')::uuid
  AND appstore.submissions.closed IS NULL
  AND appstore.submissions.deleted IS NULL;

-- name: RegistrySubmissionCloseOpen :exec
-- Withdrawal closes the round without a decision, so reviewed and
-- reviewer_user_id stay null and submissions_ck_reviewed still holds.
UPDATE appstore.submissions SET closed = now()
WHERE appstore.submissions.plugin_definition_id = sqlc.arg('plugin_definition_id')::uuid
  AND appstore.submissions.closed IS NULL
  AND appstore.submissions.deleted IS NULL;

-- name: RegistrySubmissionCloseOpenByPluginID :exec
-- DeletePlugin closes any round still open across every version of the listing.
UPDATE appstore.submissions SET closed = now()
WHERE appstore.submissions.closed IS NULL
  AND appstore.submissions.deleted IS NULL
  AND EXISTS (
	SELECT 1 FROM appstore.plugin_definitions
	 WHERE appstore.plugin_definitions.id = appstore.submissions.plugin_definition_id
	   AND appstore.plugin_definitions.plugin_id = sqlc.arg('plugin_id')::uuid
  );

-- name: RegistrySubmissionLatestByDefinitionID :one
-- Supplies both fields registry.v1.PluginVersion needs from the review record:
-- submitted, and review_feedback — the hand-off of the reviewer's note onto the
-- developer's surface (FUN-20), which is read rather than copied onto the
-- version. Latest round wins, so a note stops showing once the developer
-- resubmits and a fresh round opens.
SELECT appstore.submissions.submitted, appstore.submissions.feedback
FROM appstore.submissions
WHERE appstore.submissions.plugin_definition_id = sqlc.arg('plugin_definition_id')::uuid
  AND appstore.submissions.deleted IS NULL
ORDER BY appstore.submissions.submitted DESC
LIMIT 1;

-- name: RegistryCategoryList :many
-- Read straight from appstore rather than through catalog.v1, so publishing does
-- not depend on the public storefront being reachable.
SELECT appstore.categories.id, appstore.categories.name
FROM appstore.categories
WHERE appstore.categories.deleted IS NULL
ORDER BY appstore.categories.name ASC;
