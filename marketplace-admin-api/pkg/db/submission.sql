-- Submission queries for admin.v1.ReviewService. The admin role's policies are
-- USING (true) — a reviewer acts on submissions from every publishing
-- organization — so unlike the registry's queries these filter nothing by
-- tenancy, only by lifecycle.

-- name: SubmissionList :many
-- status is the version's own column (FUN-20: the submission table records who
-- and when, never a second copy of the state). An empty status returns every
-- round.
SELECT
	submissions.id,
	plugin_definitions.plugin_id,
	submissions.plugin_definition_id,
	plugins.organization_id,
	submissions.submitter_user_id,
	plugin_definitions.status,
	submissions.submitted,
	submissions.reviewed,
	submissions.closed,
	submissions.reviewer_user_id,
	submissions.rejection_reason,
	submissions.feedback
FROM appstore.submissions
JOIN appstore.plugin_definitions ON plugin_definitions.id = submissions.plugin_definition_id
JOIN appstore.plugins ON plugins.id = plugin_definitions.plugin_id
WHERE submissions.deleted IS NULL
  AND plugin_definitions.deleted IS NULL
  AND plugins.deleted IS NULL
  AND (sqlc.arg('status')::text = '' OR plugin_definitions.status = sqlc.arg('status')::text)
ORDER BY submissions.submitted DESC;

-- name: SubmissionGetByID :one
SELECT
	submissions.id,
	plugin_definitions.plugin_id,
	submissions.plugin_definition_id,
	plugins.organization_id,
	submissions.submitter_user_id,
	plugin_definitions.status,
	submissions.submitted,
	submissions.reviewed,
	submissions.closed,
	submissions.reviewer_user_id,
	submissions.rejection_reason,
	submissions.feedback
FROM appstore.submissions
JOIN appstore.plugin_definitions ON plugin_definitions.id = submissions.plugin_definition_id
JOIN appstore.plugins ON plugins.id = plugin_definitions.plugin_id
WHERE submissions.id = sqlc.arg('id')::uuid
  AND submissions.deleted IS NULL
  AND plugin_definitions.deleted IS NULL
  AND plugins.deleted IS NULL;

-- name: SubmissionDecide :execrows
-- Every decision closes the round in the same statement that records it, so
-- submissions_ck_reviewed and submissions_ck_closed hold. closed IS NULL is the
-- guard: zero rows means the round was decided or withdrawn underneath the
-- handler, and the caller must not report a decision it did not make.
UPDATE appstore.submissions SET
	reviewer_user_id = sqlc.arg('reviewer_user_id')::uuid,
	reviewed = now(),
	closed = now(),
	rejection_reason = sqlc.narg('rejection_reason')::text,
	feedback = sqlc.arg('feedback')::text
WHERE id = sqlc.arg('id')::uuid
  AND closed IS NULL
  AND deleted IS NULL;

-- name: PluginVersionApprove :execrows
-- Approval is the one transition that stamps published, which is what makes
-- the listing catalog-visible. The status guard mirrors the registry's
-- PluginVersionSetStatus: zero rows means the version left PENDING underneath
-- the handler.
UPDATE appstore.plugin_definitions SET
	status = 'approved',
	published = now()
WHERE id = sqlc.arg('id')::uuid
  AND status = 'pending'
  AND deleted IS NULL;

-- name: PluginVersionSetStatus :execrows
-- The non-publishing decisions: changes_requested and rejected. Only a PENDING
-- version can be decided, so the guard is a constant rather than a parameter.
UPDATE appstore.plugin_definitions SET status = sqlc.arg('status')::text
WHERE id = sqlc.arg('id')::uuid
  AND status = 'pending'
  AND deleted IS NULL;
