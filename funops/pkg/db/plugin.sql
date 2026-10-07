-- Plugin review queries. funops is a second operator path to the same
-- transitions marketplace-admin-api's ReviewService makes: only a PENDING
-- version with an open submission round can be decided, and the version's
-- status column stays authoritative (FUN-20).

-- name: PluginFindByName :many
-- Plugin names are unique per organization, not globally, so a bare name can
-- match several rows; the caller refuses the ambiguity unless an organization
-- narrows it down.
SELECT
  appstore.plugins.id,
  tenant.organizations.name AS organization_name
FROM appstore.plugins
INNER JOIN tenant.organizations ON tenant.organizations.id = appstore.plugins.organization_id
WHERE appstore.plugins.name = @name::text
  AND (@organization_name::text = '' OR tenant.organizations.name = @organization_name::text)
  AND appstore.plugins.deleted IS NULL
ORDER BY organization_name;

-- name: PluginGetByID :one
SELECT
  appstore.plugins.id,
  appstore.plugins.name,
  tenant.organizations.name AS organization_name
FROM appstore.plugins
INNER JOIN tenant.organizations ON tenant.organizations.id = appstore.plugins.organization_id
WHERE appstore.plugins.id = @id
  AND appstore.plugins.deleted IS NULL;

-- name: PluginVersionGet :one
-- The version and whether it has an open review round. submissions_uq_open
-- pins a version to at most one open round, so a boolean is the whole story.
SELECT
  appstore.plugin_definitions.id,
  appstore.plugin_definitions.status,
  (appstore.submissions.id IS NOT NULL)::boolean AS has_open_round
FROM appstore.plugin_definitions
LEFT JOIN appstore.submissions ON appstore.submissions.plugin_definition_id = appstore.plugin_definitions.id
  AND appstore.submissions.closed IS NULL
  AND appstore.submissions.deleted IS NULL
WHERE appstore.plugin_definitions.plugin_id = @plugin_id
  AND appstore.plugin_definitions.plugin_version = @plugin_version::text
  AND appstore.plugin_definitions.deleted IS NULL;

-- name: PluginVersionList :many
-- Every live version with its open round's submitted timestamp, oldest
-- submission first: the review queue in the order it should be worked.
SELECT
  tenant.organizations.name AS organization_name,
  appstore.plugins.name AS plugin_name,
  appstore.plugin_definitions.plugin_version,
  appstore.plugin_definitions.status,
  appstore.plugin_definitions.published,
  appstore.submissions.submitted
FROM appstore.plugin_definitions
INNER JOIN appstore.plugins ON appstore.plugins.id = appstore.plugin_definitions.plugin_id
INNER JOIN tenant.organizations ON tenant.organizations.id = appstore.plugins.organization_id
LEFT JOIN appstore.submissions ON appstore.submissions.plugin_definition_id = appstore.plugin_definitions.id
  AND appstore.submissions.closed IS NULL
  AND appstore.submissions.deleted IS NULL
WHERE (@status::text = '' OR appstore.plugin_definitions.status = @status::text)
  AND appstore.plugin_definitions.deleted IS NULL
  AND appstore.plugins.deleted IS NULL
ORDER BY submitted, organization_name, plugin_name, plugin_version;

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
-- Closes the version's open round in the same statement that records the
-- decision, so submissions_ck_reviewed and submissions_ck_closed hold.
-- closed IS NULL is the guard: zero rows means the round was decided or
-- withdrawn underneath the command.
UPDATE appstore.submissions
SET reviewer_user_id = @reviewer_user_id,
    reviewed = now(),
    closed = now(),
    rejection_reason = sqlc.narg('rejection_reason')::text,
    feedback = @feedback::text
WHERE plugin_definition_id = @plugin_definition_id
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
  AND deleted IS NULL;
