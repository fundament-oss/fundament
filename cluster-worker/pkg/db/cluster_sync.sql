-- Queries used by the cluster handler's Sync method.

-- name: ClusterGetForSync :one
-- Get a cluster with fields needed to build a ClusterToSync for the sync handler.
SELECT clusters.id, clusters.name, clusters.region, clusters.kubernetes_version,
       clusters.deleted, clusters.organization_id,
       organizations.name AS organization_name
FROM tenant.clusters
JOIN tenant.organizations ON organizations.id = clusters.organization_id
WHERE clusters.id = @cluster_id;

-- name: ClusterMarkSynced :exec
-- Mark cluster as synced (Gardener accepted request).
UPDATE tenant.clusters
SET
    synced = now(),
    sync_claimed_at = NULL,
    sync_error = NULL
WHERE
    id = @cluster_id;

-- name: ClusterCreateSyncSucceededEvent :one
-- Insert sync_succeeded event when Gardener accepts the manifest.
INSERT INTO
    tenant.cluster_events (cluster_id, event_type, sync_action, message)
VALUES
    (
        @cluster_id,
        'sync_succeeded',
        @sync_action,
        @message
    )
RETURNING
    id;

-- name: ClusterCreateSyncFailedEvent :one
-- Insert sync_failed event for history.
INSERT INTO
    tenant.cluster_events (
        cluster_id,
        event_type,
        sync_action,
        message,
        attempt
    )
VALUES
    (
        @cluster_id,
        'sync_failed',
        @sync_action,
        @message,
        @attempt
    )
RETURNING
    id;
