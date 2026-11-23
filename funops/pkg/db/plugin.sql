-- Plugin review queries. funops is a second operator path to the same
-- transitions marketplace-admin-api's ReviewService makes: only a PENDING
-- version with an open submission round can be decided, and the version's
-- status column stays authoritative (FUN-20).

-- name: PluginGetByName :one
-- Plugin names are unique per organization (plugins_uq_name), so a name
-- qualified by its organization is exact. The organization must be live:
-- a soft-deleted organization frees its name for reuse
-- (organizations_uq_name), so without the filter a reused name could match
-- two rows and resolve to the deleted organization's plugin.
SELECT
  appstore.plugins.id,
  appstore.plugins.name,
  tenant.organizations.name AS organization_name
FROM appstore.plugins
INNER JOIN tenant.organizations ON tenant.organizations.id = appstore.plugins.organization_id
WHERE appstore.plugins.name = @name::text
  AND tenant.organizations.name = @organization_name::text
  AND appstore.plugins.deleted IS NULL
  AND tenant.organizations.deleted IS NULL;

-- name: PluginGetByID :one
SELECT
  appstore.plugins.id,
  appstore.plugins.name,
  tenant.organizations.name AS organization_name
FROM appstore.plugins
INNER JOIN tenant.organizations ON tenant.organizations.id = appstore.plugins.organization_id
WHERE appstore.plugins.id = @id
  AND appstore.plugins.deleted IS NULL
  AND tenant.organizations.deleted IS NULL;

-- name: PluginVersionGet :one
-- The version and its open review round, if any. submissions_uq_open pins a
-- version to at most one open round; the round's id is returned so the
-- decision can be pinned to exactly the round the operator saw.
SELECT
  appstore.plugin_definitions.id,
  appstore.plugin_definitions.status,
  appstore.submissions.id AS open_submission_id
FROM appstore.plugin_definitions
LEFT JOIN appstore.submissions ON appstore.submissions.plugin_definition_id = appstore.plugin_definitions.id
  AND appstore.submissions.closed IS NULL
  AND appstore.submissions.deleted IS NULL
WHERE appstore.plugin_definitions.plugin_id = @plugin_id
  AND appstore.plugin_definitions.plugin_version = @plugin_version::text
  AND appstore.plugin_definitions.deleted IS NULL;

-- name: PluginVersionList :many
-- Every live version with its latest round's submitted timestamp, oldest
-- submission first: the review queue in the order it should be worked. The
-- latest round regardless of closed, so a decided version still shows when it
-- was submitted; versions created but never submitted stay blank and sort
-- last, by creation.
SELECT
  tenant.organizations.name AS organization_name,
  appstore.plugins.id AS plugin_id,
  appstore.plugins.name AS plugin_name,
  appstore.plugin_definitions.plugin_version,
  appstore.plugin_definitions.status,
  appstore.plugin_definitions.published,
  appstore.plugin_definitions.created AS version_created,
  (SELECT appstore.submissions.submitted
   FROM appstore.submissions
   WHERE appstore.submissions.plugin_definition_id = appstore.plugin_definitions.id
     AND appstore.submissions.deleted IS NULL
   ORDER BY appstore.submissions.submitted DESC
   LIMIT 1) AS submitted
FROM appstore.plugin_definitions
INNER JOIN appstore.plugins ON appstore.plugins.id = appstore.plugin_definitions.plugin_id
INNER JOIN tenant.organizations ON tenant.organizations.id = appstore.plugins.organization_id
WHERE (sqlc.narg('status')::text IS NULL OR appstore.plugin_definitions.status = sqlc.narg('status')::text)
  AND appstore.plugin_definitions.deleted IS NULL
  AND appstore.plugins.deleted IS NULL
  AND tenant.organizations.deleted IS NULL
ORDER BY submitted, organization_name, plugin_name, version_created;

-- name: PluginVersionApprove :execrows
-- Approval is the one transition that stamps published, which is what makes
-- the listing catalog-visible. The status guard means zero rows when the
-- version left PENDING underneath the command.
UPDATE appstore.plugin_definitions
SET status = 'approved',
    published = now()
WHERE id = @id
  AND status = 'pending'
  AND deleted IS NULL;

-- name: PluginVersionSetStatus :execrows
-- The non-publishing decisions: changes_requested and rejected. Only a PENDING
-- version can be decided, so the guard is a constant rather than a parameter.
UPDATE appstore.plugin_definitions
SET status = @status::text
WHERE id = @id
  AND status = 'pending'
  AND deleted IS NULL;

-- name: SubmissionDecide :execrows
-- Closes the round in the same statement that records the decision, so
-- submissions_ck_reviewed and submissions_ck_closed hold. Pinned to the round
-- the command loaded, with closed IS NULL as the guard: zero rows means that
-- round was decided or withdrawn underneath the command, and a verdict must
-- never land on a round the operator did not see (a withdraw and resubmit
-- opens a fresh one).
UPDATE appstore.submissions
SET reviewer_user_id = @reviewer_user_id,
    reviewed = now(),
    closed = now(),
    rejection_reason = sqlc.narg('rejection_reason')::text,
    feedback = @feedback::text
WHERE id = @id
  AND closed IS NULL
  AND deleted IS NULL;

-- name: ReviewerGetByID :one
-- Reviewers are DCIM staff (dcim.users), the store marketplace-admin-api
-- authenticates decisions against.
SELECT
  id,
  name
FROM dcim.users
WHERE id = @id
  AND deleted IS NULL;

-- name: ReviewerFindByEmail :many
-- Email carries no unique constraint in dcim.users, so several rows can
-- match; the caller refuses the ambiguity and asks for the user ID.
SELECT
  id,
  name
FROM dcim.users
WHERE lower(email) = lower(@email::text)
  AND deleted IS NULL
ORDER BY name, id;
