-- name: OutboxGetAndLock :one
-- Claims the next pending/retryable cluster outbox row.
-- Picks up all entity types: cluster, organization_user, and project_member.
-- Uses FOR NO KEY UPDATE SKIP LOCKED for concurrent worker safety.
SELECT id,
       cluster_id,
       organization_user_id,
       project_member_id,
       event,
       source,
       status,
       retries
FROM tenant.cluster_outbox
WHERE status IN ('pending', 'retrying')
  AND (retry_after IS NULL OR retry_after <= now())
ORDER BY id ASC
LIMIT 1
FOR NO KEY UPDATE SKIP LOCKED;

-- name: OutboxMarkProcessed :exec
UPDATE tenant.cluster_outbox
SET status = 'completed',
    processed = now(),
    status_info = NULL,
    retry_after = NULL
WHERE id = @id;

-- name: OutboxMarkRetry :one
-- Marks a row for retry with exponential backoff.
-- The backoff is calculated as: base_interval * 2^(retries+1), capped at max_backoff.
-- retries+1 is used because PostgreSQL evaluates expressions using the old row value,
-- but we want the delay to reflect the new retry count (incremented in the same UPDATE).
UPDATE tenant.cluster_outbox
SET retries = retries + 1,
    retry_after = now() + LEAST(
        sqlc.arg('base_interval')::interval * (1 << (retries + 1)),
        @max_backoff::interval
    ),
    status = 'retrying',
    status_info = @status_info
WHERE id = @id
RETURNING retries;

-- name: OutboxMarkFailed :exec
-- Marks a row as permanently failed after exceeding max retries.
UPDATE tenant.cluster_outbox
SET status = 'failed', failed = now(), status_info = @status_info
WHERE id = @id;

-- name: OutboxInsertReady :exec
-- Insert a 'ready' event outbox row for a cluster.
-- Used by the status worker when a shoot transitions to ready.
-- Skips insert if there's already a pending/retrying ready row for this cluster.
INSERT INTO tenant.cluster_outbox (cluster_id, event, source)
SELECT @cluster_id, 'ready', 'status'
WHERE NOT EXISTS (
    SELECT 1 FROM tenant.cluster_outbox
    WHERE tenant.cluster_outbox.cluster_id = @cluster_id
      AND tenant.cluster_outbox.event = 'ready'
      AND tenant.cluster_outbox.status IN ('pending', 'retrying')
);

-- name: OutboxInsertReconcile :exec
-- Conditionally insert a reconcile outbox row for a cluster.
-- Skips insert if the cluster already has an active (pending/retrying) row
-- or an exhausted failed row (retries >= max_retries).
INSERT INTO tenant.cluster_outbox (cluster_id, event, source)
SELECT @cluster_id, 'reconcile', 'reconcile'
WHERE NOT EXISTS (
    SELECT 1 FROM tenant.cluster_outbox
    WHERE tenant.cluster_outbox.cluster_id = @cluster_id
      AND (
          tenant.cluster_outbox.status IN ('pending', 'retrying')
          OR (tenant.cluster_outbox.status = 'failed' AND tenant.cluster_outbox.retries >= @max_retries)
      )
);
