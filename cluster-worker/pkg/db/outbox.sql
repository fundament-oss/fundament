-- name: OutboxGetAndLock :one
-- Claims the next pending/retryable outbox row.
-- Uses FOR NO KEY UPDATE SKIP LOCKED for concurrent worker safety.
SELECT id, cluster_id, namespace_id, project_member_id, project_id, event, source, status, retries
FROM tenant.cluster_outbox
WHERE status IN ('pending', 'retrying')
  AND (retry_after IS NULL OR retry_after <= now())
ORDER BY id
FOR NO KEY UPDATE SKIP LOCKED
LIMIT 1;

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

-- name: OutboxReconcileClusters :exec
-- Catches clusters whose state may not be synced to Gardener:
--   1. Trigger never fired (bug, schema mismatch, trigger disabled)
--   2. Outbox row was lost before processing
--   3. Entity was modified after its last completed sync
--   4. First deploy / backfill (entities predate the outbox system)
-- Skips entities that already have an in-flight or permanently failed row.
-- Failed rows require manual intervention; re-enqueueing them would create
-- an infinite retry loop.
INSERT INTO tenant.cluster_outbox (cluster_id, event, source)
SELECT tenant.clusters.id, 'reconcile', 'reconcile'
FROM tenant.clusters
LEFT JOIN tenant.cluster_outbox ON tenant.cluster_outbox.cluster_id = tenant.clusters.id
  AND tenant.cluster_outbox.status = 'completed'
  AND tenant.cluster_outbox.processed >= GREATEST(
      tenant.clusters.created,
      COALESCE(tenant.clusters.deleted, '1970-01-01')
  )
WHERE tenant.cluster_outbox.id IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM tenant.cluster_outbox
    WHERE tenant.cluster_outbox.cluster_id = tenant.clusters.id
      AND tenant.cluster_outbox.status IN ('pending', 'retrying', 'failed')
  );

-- name: OutboxReconcileNamespaces :exec
-- Same logic as OutboxReconcileClusters, applied to namespaces.
INSERT INTO tenant.cluster_outbox (namespace_id, event, source)
SELECT tenant.namespaces.id, 'reconcile', 'reconcile'
FROM tenant.namespaces
LEFT JOIN tenant.cluster_outbox ON tenant.cluster_outbox.namespace_id = tenant.namespaces.id
  AND tenant.cluster_outbox.status = 'completed'
  AND tenant.cluster_outbox.processed >= GREATEST(
      tenant.namespaces.created,
      COALESCE(tenant.namespaces.deleted, '1970-01-01')
  )
WHERE tenant.cluster_outbox.id IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM tenant.cluster_outbox
    WHERE tenant.cluster_outbox.namespace_id = tenant.namespaces.id
      AND tenant.cluster_outbox.status IN ('pending', 'retrying', 'failed')
  );

-- name: OutboxReconcileProjectMembers :exec
-- Same logic as OutboxReconcileClusters, applied to project members.
INSERT INTO tenant.cluster_outbox (project_member_id, event, source)
SELECT tenant.project_members.id, 'reconcile', 'reconcile'
FROM tenant.project_members
LEFT JOIN tenant.cluster_outbox ON tenant.cluster_outbox.project_member_id = tenant.project_members.id
  AND tenant.cluster_outbox.status = 'completed'
  AND tenant.cluster_outbox.processed >= GREATEST(
      tenant.project_members.created,
      COALESCE(tenant.project_members.deleted, '1970-01-01')
  )
WHERE tenant.cluster_outbox.id IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM tenant.cluster_outbox
    WHERE tenant.cluster_outbox.project_member_id = tenant.project_members.id
      AND tenant.cluster_outbox.status IN ('pending', 'retrying', 'failed')
  );

-- name: OutboxReconcileProjects :exec
-- Same logic as OutboxReconcileClusters, but only reconciles deleted projects.
-- Active project state is managed via project_members; deletion cleanup
-- (e.g. revoking Gardener access) is the only project-level operation the
-- cluster worker needs to perform.
INSERT INTO tenant.cluster_outbox (project_id, event, source)
SELECT tenant.projects.id, 'reconcile', 'reconcile'
FROM tenant.projects
WHERE tenant.projects.deleted IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM tenant.cluster_outbox
    WHERE tenant.cluster_outbox.project_id = tenant.projects.id
      AND tenant.cluster_outbox.status = 'completed'
      AND tenant.cluster_outbox.processed >= tenant.projects.deleted
  )
  AND NOT EXISTS (
    SELECT 1 FROM tenant.cluster_outbox
    WHERE tenant.cluster_outbox.project_id = tenant.projects.id
      AND tenant.cluster_outbox.status IN ('pending', 'retrying', 'failed')
  );
