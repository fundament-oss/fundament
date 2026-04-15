-- name: ClusterCreateSyncSucceededEvent :one
-- Insert sync_succeeded event when Gardener accepts the manifest.
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

-- name: ClusterListActive :many
-- Used by periodic reconciliation to compare with Gardener state.
SELECT
    tenant.clusters.id,
    tenant.clusters.name,
    tenant.clusters.deleted,
    tenant.organizations.name AS organization_name,
    COALESCE(tenant.clusters.outbox_status = 'completed', false)::boolean AS has_completed_outbox
FROM
    tenant.clusters
    JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
WHERE
    tenant.clusters.deleted IS NULL
ORDER BY
    tenant.clusters.id;

-- name: NodePoolListByClusterID :many
-- Fetch active (non-deleted) node pools for a cluster.
-- Used by the cluster handler to build Gardener worker groups.
SELECT
    tenant.node_pools.id,
    tenant.node_pools.name,
    tenant.node_pools.machine_type,
    tenant.node_pools.autoscale_min,
    tenant.node_pools.autoscale_max,
    tenant.node_pools.created
FROM
    tenant.node_pools
WHERE
    tenant.node_pools.cluster_id = @cluster_id
    AND tenant.node_pools.deleted IS NULL
ORDER BY
    tenant.node_pools.created,
    tenant.node_pools.id;

-- name: NodePoolGetClusterID :one
-- Returns the cluster_id for a node pool (including soft-deleted node pools).
-- Used by the cluster handler to resolve node_pool_id → cluster_id.
SELECT
    tenant.node_pools.cluster_id
FROM
    tenant.node_pools
WHERE
    tenant.node_pools.id = @node_pool_id;

-- name: ClusterHasEverBeenSynced :one
-- Returns whether a cluster has been successfully synced to Gardener at least once.
-- Checks outbox history directly (EXISTS on completed rows with cluster_id set).
-- This remains true even if the latest outbox row is retrying or failed.
SELECT EXISTS (
    SELECT 1
    FROM tenant.cluster_outbox
    WHERE tenant.cluster_outbox.cluster_id = @cluster_id
      AND tenant.cluster_outbox.status = 'completed'
)::boolean AS has_been_synced;

-- name: ClusterListNeedingStatusCheck :many
-- Get clusters where we need to check Gardener status (active clusters).
-- Polls clusters in non-terminal states: NULL (never checked), pending,
-- progressing, error.
SELECT
    tenant.clusters.id,
    tenant.clusters.name,
    tenant.clusters.region,
    tenant.clusters.kubernetes_version,
    tenant.clusters.deleted,
    tenant.clusters.shoot_status,
    tenant.clusters.organization_id,
    tenant.clusters.shoot_status_updated,
    tenant.organizations.name AS organization_name
FROM
    tenant.clusters
    JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
WHERE
    ( -- Cluster has been synced: has shoot_status or a completed outbox row
        tenant.clusters.shoot_status IS NOT NULL
        OR tenant.clusters.outbox_status = 'completed'
    )
    AND tenant.clusters.deleted IS NULL -- Active (not deleted)
    AND (
        tenant.clusters.shoot_status IS NULL -- Never checked
        OR tenant.clusters.shoot_status = 'pending' -- Shoot not yet visible in Gardener
        OR tenant.clusters.shoot_status = 'progressing' -- Gardener creating/updating
        OR tenant.clusters.shoot_status = 'error'
    ) -- Failed, might recover
    AND (
        tenant.clusters.shoot_status_updated IS NULL -- Never checked
        OR tenant.clusters.shoot_status_updated < now() - INTERVAL '30 seconds'
    ) -- Not checked recently
ORDER BY
    shoot_status_updated NULLS FIRST
LIMIT
    @limit_count;

-- name: ClusterListDeletedNeedingVerification :many
-- Get deleted clusters where we need to verify Shoot is actually gone from Gardener.
-- Polls until shoot_status = 'deleted' (confirmed removed).
SELECT
    tenant.clusters.id,
    tenant.clusters.name,
    tenant.clusters.region,
    tenant.clusters.kubernetes_version,
    tenant.clusters.deleted,
    tenant.clusters.shoot_status,
    tenant.clusters.organization_id,
    tenant.clusters.shoot_status_updated,
    tenant.organizations.name AS organization_name
FROM
    tenant.clusters
    JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
WHERE
    ( -- Delete has been synced: has shoot_status or a completed outbox row
        tenant.clusters.shoot_status IS NOT NULL
        OR tenant.clusters.outbox_status = 'completed'
    )
    AND tenant.clusters.deleted IS NOT NULL -- Soft-deleted
    AND (
        tenant.clusters.shoot_status IS NULL
        OR tenant.clusters.shoot_status != 'deleted'
    ) -- Not yet confirmed deleted
    AND (
        tenant.clusters.shoot_status_updated IS NULL
        OR tenant.clusters.shoot_status_updated < now() - INTERVAL '30 seconds'
    )
ORDER BY
    shoot_status_updated NULLS FIRST
LIMIT
    @limit_count;

-- name: ClusterUpdateShootStatus :exec
-- Update shoot status from Gardener polling.
UPDATE tenant.clusters
SET
    shoot_status = @status,
    shoot_status_message = @message,
    shoot_status_updated = now()
WHERE
    id = @cluster_id;

-- name: ClusterCreateStatusEvent :one
-- Insert status event (only for milestone states: ready, error, deleted).
INSERT INTO
    tenant.cluster_events (cluster_id, event_type, message)
VALUES
    (@cluster_id, @event_type, @message)
RETURNING
    id;
