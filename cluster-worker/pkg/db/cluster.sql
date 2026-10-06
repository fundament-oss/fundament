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
-- The catalog join resolves the machine-type name when the pool references
-- region_machine_types; legacy pools fall back to the machine_type text column.
SELECT
    tenant.node_pools.id,
    tenant.node_pools.name,
    COALESCE(catalog.machine_types.name, tenant.node_pools.machine_type) AS machine_type,
    tenant.node_pools.autoscale_min,
    tenant.node_pools.autoscale_max,
    tenant.node_pools.created
FROM
    tenant.node_pools
    LEFT JOIN catalog.region_machine_types ON catalog.region_machine_types.id = tenant.node_pools.region_machine_type_id
    LEFT JOIN catalog.machine_types ON catalog.machine_types.id = catalog.region_machine_types.machine_type_id
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

-- name: ClusterListForStatusSweep :many
-- IDs of every cluster the status sweep checks: it has reached Gardener (a
-- status was recorded or an outbox row completed) and, if soft-deleted, its
-- Shoot is not confirmed gone yet.
SELECT
    tenant.clusters.id
FROM
    tenant.clusters
WHERE
    (
        tenant.clusters.shoot_status IS NOT NULL
        OR tenant.clusters.outbox_status = 'completed'
    )
    AND (
        tenant.clusters.deleted IS NULL
        OR tenant.clusters.shoot_status IS DISTINCT FROM 'deleted'
    )
ORDER BY
    tenant.clusters.id;

-- name: ClusterGetForStatusCheck :one
-- Load one cluster for a status check, active or soft-deleted. synced says
-- whether the cluster has reached Gardener at all (a status was recorded or
-- an outbox row completed); before that there is no Shoot to check.
SELECT
    tenant.clusters.id,
    tenant.clusters.name,
    tenant.clusters.region,
    tenant.clusters.kubernetes_version,
    tenant.clusters.deleted,
    tenant.clusters.shoot_status,
    tenant.clusters.shoot_status_message,
    tenant.clusters.shoot_health,
    tenant.clusters.shoot_updating,
    tenant.clusters.organization_id,
    tenant.clusters.shoot_status_updated,
    tenant.organizations.name AS organization_name,
    catalog.regions.cloud_profile,
    catalog.regions.cloud_profile_region,
    (
        tenant.clusters.shoot_status IS NOT NULL
        OR tenant.clusters.outbox_status = 'completed'
    )::boolean AS synced
FROM
    tenant.clusters
    JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
    LEFT JOIN catalog.regions ON catalog.regions.id = tenant.clusters.region_id
WHERE
    tenant.clusters.id = @cluster_id;

-- name: ClusterUpdateShootStatus :execrows
-- Update shoot status from Gardener polling. health is NULL unless ready;
-- updating is set while an update fundament pushed rolls out. Writes nothing
-- when the row changed since it was read (checked_at is the
-- shoot_status_updated read then), such as the sync handler marking an update.
UPDATE tenant.clusters
SET
    shoot_status = @status,
    shoot_status_message = @message,
    shoot_health = @health,
    shoot_updating = @updating,
    shoot_status_updated = now()
WHERE
    id = @cluster_id
    AND shoot_status_updated IS NOT DISTINCT FROM @checked_at;

-- name: ClusterMarkShootUpdating :execrows
-- Mark a ready cluster as updating once Gardener accepted a spec change. It
-- stays ready, so what is gated on a ready cluster (kubeconfig, member sync)
-- keeps working while the update rolls out.
UPDATE tenant.clusters
SET
    shoot_updating = true,
    shoot_status_message = @message,
    shoot_status_updated = now()
WHERE
    id = @cluster_id
    AND shoot_status = 'ready';

-- name: ClusterGetLastWarningMessage :one
-- The message of the cluster's most recent status_warning event since its last
-- milestone (ready, healthy, error or lost), so an error Gardener retries again
-- and again is recorded once, and the same error in a later incident is
-- recorded again.
SELECT
    tenant.cluster_events.message
FROM
    tenant.cluster_events
WHERE
    tenant.cluster_events.cluster_id = @cluster_id
    AND tenant.cluster_events.event_type = 'status_warning'
    AND tenant.cluster_events.created > COALESCE((
        SELECT
            max(tenant.cluster_events.created)
        FROM
            tenant.cluster_events
        WHERE
            tenant.cluster_events.cluster_id = @cluster_id
            AND tenant.cluster_events.event_type IN ('status_ready', 'status_healthy', 'status_error', 'status_lost')
    ), '-infinity'::timestamptz)
ORDER BY
    tenant.cluster_events.created DESC
LIMIT
    1;

-- name: ClusterCreateStatusEvent :one
-- Insert status event (only for milestone states: ready, error, deleted).
INSERT INTO
    tenant.cluster_events (cluster_id, event_type, message)
VALUES
    (@cluster_id, @event_type, @message)
RETURNING
    id;
