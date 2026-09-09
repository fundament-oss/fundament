-- Version and submission queries for registry.v1.PublicationService.
--
-- Every version query joins through appstore.plugins even where plugin_id alone
-- would find the row. plugin_definitions' own policy gates on ownership through
-- that join already, but writing it explicitly keeps a query that is later
-- changed to select by version id from silently widening its scope.

-- name: PluginVersionListByPluginID :many
SELECT
	plugin_definitions.id,
	plugin_definitions.plugin_id,
	plugin_definitions.plugin_version,
	plugin_definitions.manifest,
	plugin_definitions.hash,
	plugin_definitions.status,
	plugin_definitions.release_notes,
	plugin_definitions.created,
	plugin_definitions.published
FROM appstore.plugin_definitions
JOIN appstore.plugins ON plugins.id = plugin_definitions.plugin_id
WHERE plugin_definitions.plugin_id = sqlc.arg('plugin_id')::uuid
  AND plugin_definitions.deleted IS NULL
  AND plugins.deleted IS NULL
ORDER BY plugin_definitions.created DESC;

-- name: PluginVersionGetByID :one
SELECT
	plugin_definitions.id,
	plugin_definitions.plugin_id,
	plugin_definitions.plugin_version,
	plugin_definitions.manifest,
	plugin_definitions.hash,
	plugin_definitions.status,
	plugin_definitions.release_notes,
	plugin_definitions.created,
	plugin_definitions.published
FROM appstore.plugin_definitions
JOIN appstore.plugins ON plugins.id = plugin_definitions.plugin_id
WHERE plugin_definitions.id = sqlc.arg('id')::uuid
  AND plugin_definitions.deleted IS NULL
  AND plugins.deleted IS NULL;

-- name: PluginVersionCreate :one
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
RETURNING id, created;

-- name: PluginVersionSetStatus :execrows
-- No join to plugins: an UPDATE cannot reference it without a FROM clause, and
-- the policy already gates ownership.
--
-- expected_status is the status the handler's read saw, on another pooled
-- connection. Without it a withdrawal racing an approval would un-publish the
-- approved version; zero rows means the status moved underneath the handler.
UPDATE appstore.plugin_definitions SET status = sqlc.arg('status')::text
WHERE id = sqlc.arg('id')::uuid
  AND status = sqlc.arg('expected_status')::text
  AND deleted IS NULL;

-- name: PluginVersionsSoftDeleteByPluginID :exec
-- DeletePlugin cascades: a soft-deleted listing must not strand active versions.
UPDATE appstore.plugin_definitions SET deleted = now()
WHERE plugin_id = sqlc.arg('plugin_id')::uuid
  AND deleted IS NULL;

-- name: SubmissionCreate :one
INSERT INTO appstore.submissions (plugin_definition_id, submitter_user_id)
VALUES (sqlc.arg('plugin_definition_id')::uuid, sqlc.arg('submitter_user_id')::uuid)
RETURNING id;

-- name: SubmissionOpenByDefinitionID :one
SELECT id, submitted
FROM appstore.submissions
WHERE plugin_definition_id = sqlc.arg('plugin_definition_id')::uuid
  AND closed IS NULL
  AND deleted IS NULL;

-- name: SubmissionCloseOpen :execrows
-- Withdrawal closes the round without a decision, so reviewed and
-- reviewer_user_id stay null and submissions_ck_reviewed still holds.
--
-- :execrows for the same reason PluginVersionSetStatus is: the status write
-- above says the version was PENDING, so a round must be open. Zero rows means
-- it was closed underneath the handler, and withdrawing without closing
-- anything would report success for half a transition.
UPDATE appstore.submissions SET closed = now()
WHERE plugin_definition_id = sqlc.arg('plugin_definition_id')::uuid
  AND closed IS NULL
  AND deleted IS NULL;

-- name: SubmissionCloseOpenByPluginID :exec
-- DeletePlugin closes any round still open across every version of the listing.
UPDATE appstore.submissions SET closed = now()
WHERE closed IS NULL
  AND deleted IS NULL
  AND EXISTS (
	SELECT 1 FROM appstore.plugin_definitions
	 WHERE plugin_definitions.id = submissions.plugin_definition_id
	   AND plugin_definitions.plugin_id = sqlc.arg('plugin_id')::uuid
  );

-- name: SubmissionLatestByDefinitionIDs :many
-- Supplies both fields registry.v1.PluginVersion needs from the review record:
-- submitted, and review_feedback — the hand-off of the reviewer's note onto the
-- developer's surface (FUN-20), which is read rather than copied onto the
-- version. Latest round wins, so a note stops showing once the developer
-- resubmits and a fresh round opens.
SELECT DISTINCT ON (plugin_definition_id)
	plugin_definition_id,
	submitted,
	feedback
FROM appstore.submissions
WHERE plugin_definition_id = ANY(sqlc.arg('plugin_definition_ids')::uuid[])
  AND deleted IS NULL
ORDER BY plugin_definition_id ASC, submitted DESC;

-- name: CategoryList :many
-- Read straight from appstore rather than through catalog.v1, so publishing does
-- not depend on the public storefront being reachable.
SELECT id, name
FROM appstore.categories
WHERE deleted IS NULL
ORDER BY name ASC;
