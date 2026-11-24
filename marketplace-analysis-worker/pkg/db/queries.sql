-- Queries for marketplace-analysis-worker (FUN-24). The analysis table is both
-- the queue and the result store, one row per plugin version.

-- name: AnalysisClaim :one
-- Claims the oldest claimable row and leases it: a running row whose lease
-- (retry_after) has passed belonged to a worker that died mid-analysis, so it
-- is claimable again, and counts as a failed attempt. That keeps an image that
-- crashes the worker every time from being retried forever. The claim commits
-- at once; the analysis itself runs outside any transaction.
--
-- The lease's end doubles as the claim's token: every query that records the
-- outcome matches on it, so a worker whose lease ran out cannot overwrite the
-- attempt that claimed the row after it.
--
-- A deleted version is skipped, in the subquery so its row cannot hold up the
-- rows behind it. A rejected or withdrawn version is still analyzed: its round
-- stays in the history a reviewer opens.
UPDATE appstore.plugin_image_analyses SET
	status = 'running',
	retries = appstore.plugin_image_analyses.retries
		+ CASE WHEN appstore.plugin_image_analyses.status = 'running' THEN 1 ELSE 0 END,
	retry_after = now() + sqlc.arg('lease')::interval
FROM appstore.plugin_definitions
WHERE appstore.plugin_image_analyses.id = (
	SELECT appstore.plugin_image_analyses.id
	FROM appstore.plugin_image_analyses
	WHERE appstore.plugin_image_analyses.deleted IS NULL
	  AND appstore.plugin_image_analyses.status IN ('pending', 'running', 'retrying')
	  AND (appstore.plugin_image_analyses.retry_after IS NULL OR appstore.plugin_image_analyses.retry_after <= now())
	  AND EXISTS (
		SELECT 1
		FROM appstore.plugin_definitions
		WHERE appstore.plugin_definitions.id = appstore.plugin_image_analyses.plugin_definition_id
		  AND appstore.plugin_definitions.deleted IS NULL
	  )
	ORDER BY appstore.plugin_image_analyses.created ASC
	LIMIT 1
	FOR NO KEY UPDATE SKIP LOCKED
)
  AND appstore.plugin_definitions.id = appstore.plugin_image_analyses.plugin_definition_id
RETURNING
	appstore.plugin_image_analyses.id,
	appstore.plugin_image_analyses.plugin_definition_id,
	appstore.plugin_image_analyses.retries,
	appstore.plugin_image_analyses.retry_after,
	appstore.plugin_definitions.image;

-- name: AnalysisComplete :exec
UPDATE appstore.plugin_image_analyses SET
	status = 'completed',
	status_info = NULL,
	retry_after = NULL,
	image_digest = sqlc.arg('image_digest')::text,
	platform = sqlc.arg('platform')::text,
	platforms = sqlc.arg('platforms')::jsonb,
	packages = sqlc.arg('packages')::jsonb,
	files = sqlc.arg('files')::jsonb,
	files_truncated = sqlc.arg('files_truncated')::boolean,
	sbom = sqlc.arg('sbom')::bytea,
	analyzed = now()
WHERE id = sqlc.arg('id')::uuid
  AND status = 'running'
  AND retry_after = sqlc.arg('leased_until')::timestamptz;

-- name: AnalysisRetry :exec
-- Schedules another attempt with exponential backoff: base * 2^retries,
-- capped at max_backoff.
UPDATE appstore.plugin_image_analyses SET
	status = 'retrying',
	status_info = sqlc.arg('status_info')::text,
	retries = retries + 1,
	retry_after = now() + LEAST(
		sqlc.arg('base_backoff')::interval * (1 << retries),
		sqlc.arg('max_backoff')::interval
	)
WHERE id = sqlc.arg('id')::uuid
  AND status = 'running'
  AND retry_after = sqlc.arg('leased_until')::timestamptz;

-- name: AnalysisFail :exec
-- A failure retrying cannot fix. A new submission round resets the row to
-- pending (appstore.plugin_image_analysis_enqueue).
UPDATE appstore.plugin_image_analyses SET
	status = 'failed',
	status_info = sqlc.arg('status_info')::text,
	platforms = sqlc.arg('platforms')::jsonb,
	retry_after = NULL
WHERE id = sqlc.arg('id')::uuid
  AND status = 'running'
  AND retry_after = sqlc.arg('leased_until')::timestamptz;

-- name: AnalysisRelease :exec
-- Hands a row back untouched when the worker shuts down mid-analysis, so the
-- next worker picks it up without waiting for the lease or counting an attempt.
UPDATE appstore.plugin_image_analyses SET
	status = CASE WHEN retries > 0 THEN 'retrying' ELSE 'pending' END,
	retry_after = NULL
WHERE id = sqlc.arg('id')::uuid
  AND status = 'running'
  AND retry_after = sqlc.arg('leased_until')::timestamptz;

-- name: AnalysisEnqueueMissing :execrows
-- The sweep: every version a reviewer may look at, or compare with, that has
-- no analysis yet. Backfills the versions that predate the analysis, and any
-- submission whose trigger insert was lost.
INSERT INTO appstore.plugin_image_analyses (plugin_definition_id)
SELECT appstore.plugin_definitions.id
FROM appstore.plugin_definitions
WHERE appstore.plugin_definitions.status IN ('pending', 'approved')
  AND appstore.plugin_definitions.deleted IS NULL
  AND NOT EXISTS (
	SELECT 1
	FROM appstore.plugin_image_analyses
	WHERE appstore.plugin_image_analyses.plugin_definition_id = appstore.plugin_definitions.id
	  AND appstore.plugin_image_analyses.deleted IS NULL
  )
ON CONFLICT DO NOTHING;
