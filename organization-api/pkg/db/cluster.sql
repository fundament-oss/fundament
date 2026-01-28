
-- name: ClusterListByOrganizationID :many
-- List active clusters and clusters being deleted (not yet confirmed deleted in Gardener).
-- Excludes clusters where Gardener has confirmed deletion (shoot_status = 'deleted').
SELECT id, organization_id, name, region, kubernetes_version, created, deleted,
       synced, sync_error, sync_attempts, shoot_status, shoot_status_message, shoot_status_updated
FROM tenant.clusters
WHERE organization_id = $1
  AND (deleted IS NULL OR shoot_status IS DISTINCT FROM 'deleted')
ORDER BY created DESC;

-- name: ClusterGetByID :one
-- Get cluster by ID, including deleted clusters for direct access.
SELECT id, organization_id, name, region, kubernetes_version, created, deleted,
       synced, sync_error, sync_attempts, shoot_status, shoot_status_message, shoot_status_updated
FROM tenant.clusters
WHERE id = $1;

-- name: ClusterCreate :one
-- Create a cluster if no active or pending-delete cluster with the same name exists.
-- Allows creation only after delete is finalized (synced to Gardener).
-- Returns NULL if blocked (caller should check for pgx.ErrNoRows).
INSERT INTO tenant.clusters (organization_id, name, region, kubernetes_version)
SELECT $1, $2, $3, $4
WHERE NOT EXISTS (
    SELECT 1
    FROM tenant.clusters
    WHERE organization_id = $1
      AND name = $2
      AND (deleted IS NULL OR synced IS NULL)
)
RETURNING id;

-- name: ClusterUpdate :execrows
UPDATE tenant.clusters
SET kubernetes_version = COALESCE(sqlc.narg('kubernetes_version'), kubernetes_version)
WHERE id = $1 AND deleted IS NULL;

-- name: ClusterDelete :execrows
UPDATE tenant.clusters
SET deleted = NOW()
WHERE id = $1 AND deleted IS NULL;

-- name: ClusterGetEvents :many
-- Get event history for a cluster
SELECT id, cluster_id, event_type, created, sync_action, message, attempt
FROM tenant.cluster_events
WHERE cluster_id = $1
ORDER BY created DESC, id DESC
LIMIT $2;

-- name: ClusterCreateSyncRequestedEvent :exec
-- Insert sync_requested event when cluster is created/updated.
INSERT INTO tenant.cluster_events (cluster_id, event_type, sync_action)
VALUES ($1, 'sync_requested', $2);

