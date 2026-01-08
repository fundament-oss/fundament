-- name: ClaimUnsyncedCluster :one
-- Claims the oldest unsynced cluster for processing (atomic with SKIP LOCKED).
-- Includes deleted clusters so worker can sync deletions to Gardener.
-- Respects backoff: only claim if enough time has passed since last attempt.
-- Backoff formula: 30s * 2^attempts, capped at 15 minutes (900s).
SELECT
    c.id,
    c.name,
    c.deleted,
    cs.sync_attempts,
    t.name as tenant_name
FROM organization.clusters c
JOIN organization.cluster_sync cs ON cs.cluster_id = c.id
JOIN organization.tenants t ON t.id = c.tenant_id
WHERE cs.synced IS NULL
  AND (
    cs.sync_last_attempt IS NULL
    OR cs.sync_last_attempt + (LEAST(30 * POWER(2, cs.sync_attempts), 900) * INTERVAL '1 second') < now()
  )
ORDER BY c.created
FOR NO KEY UPDATE OF cs SKIP LOCKED
LIMIT 1;

-- name: MarkClusterSynced :exec
-- Called on successful sync - resets error tracking.
UPDATE organization.cluster_sync
SET synced = now(),
    sync_error = NULL,
    sync_attempts = 0,
    sync_last_attempt = now()
WHERE cluster_id = $1;

-- name: MarkClusterSyncFailed :exec
-- Called on failed sync - tracks error and increments attempt count.
UPDATE organization.cluster_sync
SET sync_error = $2,
    sync_attempts = sync_attempts + 1,
    sync_last_attempt = now()
WHERE cluster_id = $1;

-- name: ResetClusterSynced :exec
-- Used by reconciliation to mark a cluster for re-sync.
UPDATE organization.cluster_sync
SET synced = NULL
WHERE cluster_id = $1;

-- name: ListActiveClusters :many
-- Used by periodic reconciliation to compare with Gardener state.
SELECT
    c.id,
    c.name,
    c.deleted,
    cs.synced,
    t.name as tenant_name
FROM organization.clusters c
JOIN organization.cluster_sync cs ON cs.cluster_id = c.id
JOIN organization.tenants t ON t.id = c.tenant_id
WHERE c.deleted IS NULL;

-- name: ListFailingClusters :many
-- Used for alerting - clusters that have failed multiple times.
SELECT
    c.id,
    c.name,
    cs.sync_error,
    cs.sync_attempts,
    t.name as tenant_name
FROM organization.clusters c
JOIN organization.cluster_sync cs ON cs.cluster_id = c.id
JOIN organization.tenants t ON t.id = c.tenant_id
WHERE cs.sync_attempts >= $1;

-- name: ListClustersNeedingStatusCheck :many
-- Get clusters where we need to check Gardener status (active clusters).
SELECT c.id, c.name, c.deleted, t.name as tenant_name
FROM organization.clusters c
JOIN organization.cluster_sync cs ON cs.cluster_id = c.id
JOIN organization.tenants t ON t.id = c.tenant_id
WHERE cs.synced IS NOT NULL                           -- Manifest was applied
  AND c.deleted IS NULL                               -- Active (not deleted)
  AND (cs.shoot_status IS NULL                        -- Never checked
       OR cs.shoot_status = 'progressing')            -- Still in progress
  AND (cs.shoot_status_updated IS NULL                -- Never checked
       OR cs.shoot_status_updated < now() - INTERVAL '30 seconds')  -- Not checked recently
ORDER BY cs.shoot_status_updated NULLS FIRST
LIMIT $1;

-- name: ListDeletedClustersNeedingVerification :many
-- Get deleted clusters where we need to verify Shoot is actually gone.
SELECT c.id, c.name, c.deleted, t.name as tenant_name
FROM organization.clusters c
JOIN organization.cluster_sync cs ON cs.cluster_id = c.id
JOIN organization.tenants t ON t.id = c.tenant_id
WHERE cs.synced IS NOT NULL                           -- Delete was synced
  AND c.deleted IS NOT NULL                           -- Soft-deleted
  AND (cs.shoot_status IS NULL OR cs.shoot_status != 'deleted') -- Not yet confirmed deleted
  AND (cs.shoot_status_updated IS NULL
       OR cs.shoot_status_updated < now() - INTERVAL '30 seconds')
ORDER BY cs.shoot_status_updated NULLS FIRST
LIMIT $1;

-- name: UpdateShootStatus :exec
-- Update shoot status from Gardener polling.
UPDATE organization.cluster_sync
SET shoot_status = $2,
    shoot_status_message = $3,
    shoot_status_updated = now()
WHERE cluster_id = $1;

-- name: GetClusterByID :one
-- Get a single cluster by ID with sync state (for testing).
SELECT
    c.id,
    c.name,
    c.deleted,
    cs.synced,
    cs.sync_error,
    cs.sync_attempts,
    cs.sync_last_attempt,
    cs.shoot_status,
    cs.shoot_status_message,
    cs.shoot_status_updated,
    t.name as tenant_name
FROM organization.clusters c
JOIN organization.cluster_sync cs ON cs.cluster_id = c.id
JOIN organization.tenants t ON t.id = c.tenant_id
WHERE c.id = $1;

-- name: GetClusterSyncState :one
-- Get just the sync state for a cluster.
SELECT
    cluster_id,
    synced,
    sync_error,
    sync_attempts,
    sync_last_attempt,
    shoot_status,
    shoot_status_message,
    shoot_status_updated
FROM organization.cluster_sync
WHERE cluster_id = $1;
