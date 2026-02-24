-- name: OutboxGetAndLock :one
-- Claims the next pending/retryable outbox row.
-- Uses FOR NO KEY UPDATE SKIP LOCKED for concurrent worker safety.
SELECT id,
       subject_id,
       entity_type,
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
SET status = 'completed', processed = now()
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


