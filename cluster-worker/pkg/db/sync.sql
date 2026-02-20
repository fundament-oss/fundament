-- name: ClusterMarkSynced :exec
-- Mark cluster as synced (Gardener accepted the manifest).
UPDATE tenant.clusters
SET
    synced = now()
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
        message
    )
VALUES
    (
        @cluster_id,
        'sync_failed',
        @sync_action,
        @message
    )
RETURNING
    id;

-- name: ClusterListActiveIDs :many
-- List IDs of all non-deleted clusters (for orphan detection).
SELECT
    tenant.clusters.id
FROM
    tenant.clusters
WHERE
    tenant.clusters.deleted IS NULL;

-- name: ClusterGetForSync :one
-- Get a single cluster by ID with all data needed for sync.
SELECT
    tenant.clusters.id,
    tenant.clusters.name,
    tenant.clusters.region,
    tenant.clusters.kubernetes_version,
    tenant.clusters.deleted,
    tenant.clusters.organization_id,
    tenant.organizations.name AS organization_name
FROM
    tenant.clusters
    JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
WHERE
    tenant.clusters.id = @cluster_id;
