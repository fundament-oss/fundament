-- name: ClusterList :many
-- List active clusters and clusters being deleted (not yet confirmed deleted in Gardener).
-- Excludes clusters where Gardener has confirmed deletion (shoot_status = 'deleted').
-- The sync state is the cluster's own latest sync row (view cluster_sync_state);
-- a node pool whose own sync failed is named separately with its error (view
-- cluster_node_pool_sync_failure), so it never stands in for the cluster's state.
SELECT
    tenant.clusters.id,
    tenant.clusters.organization_id,
    tenant.clusters.name,
    tenant.clusters.region,
    tenant.clusters.kubernetes_version,
    tenant.clusters.created,
    tenant.clusters.deleted,
    tenant.clusters.shoot_status,
    tenant.clusters.shoot_status_message,
    tenant.clusters.shoot_status_updated,
    tenant.clusters.shoot_updating,
    tenant.cluster_sync_state.outbox_status,
    COALESCE(tenant.cluster_sync_state.outbox_retries, 0)::integer AS outbox_retries,
    tenant.cluster_sync_state.outbox_error,
    tenant.cluster_node_pool_sync_failure.node_pool_name AS failed_node_pool_name,
    tenant.cluster_node_pool_sync_failure.outbox_error AS failed_node_pool_error,
    (SELECT COUNT(*)
     FROM tenant.projects
     WHERE projects.cluster_id = clusters.id AND projects.deleted IS NULL) AS project_count,
    (SELECT COUNT(*)
     FROM tenant.node_pools
     WHERE node_pools.cluster_id = clusters.id AND node_pools.deleted IS NULL) AS node_pool_count
FROM tenant.clusters
    LEFT JOIN tenant.cluster_sync_state ON tenant.cluster_sync_state.cluster_id = tenant.clusters.id
    LEFT JOIN tenant.cluster_node_pool_sync_failure ON tenant.cluster_node_pool_sync_failure.cluster_id = tenant.clusters.id
WHERE (tenant.clusters.deleted IS NULL OR tenant.clusters.shoot_status IS DISTINCT FROM 'deleted')
ORDER BY tenant.clusters.created DESC, tenant.clusters.id DESC;

-- name: ClusterGetByID :one
-- Get cluster by ID, including deleted clusters for direct access. Sync state
-- as in ClusterList.
SELECT
    tenant.clusters.id,
    tenant.clusters.organization_id,
    tenant.clusters.name,
    tenant.clusters.region,
    tenant.clusters.kubernetes_version,
    tenant.clusters.created,
    tenant.clusters.deleted,
    tenant.clusters.shoot_status,
    tenant.clusters.shoot_status_message,
    tenant.clusters.shoot_status_updated,
    tenant.clusters.shoot_updating,
    tenant.cluster_sync_state.outbox_status,
    COALESCE(tenant.cluster_sync_state.outbox_retries, 0)::integer AS outbox_retries,
    tenant.cluster_sync_state.outbox_error,
    tenant.cluster_node_pool_sync_failure.node_pool_name AS failed_node_pool_name,
    tenant.cluster_node_pool_sync_failure.outbox_error AS failed_node_pool_error
FROM tenant.clusters
    LEFT JOIN tenant.cluster_sync_state ON tenant.cluster_sync_state.cluster_id = tenant.clusters.id
    LEFT JOIN tenant.cluster_node_pool_sync_failure ON tenant.cluster_node_pool_sync_failure.cluster_id = tenant.clusters.id
WHERE tenant.clusters.id = $1;

-- name: ClusterGetByName :one
-- Sync state as in ClusterList.
SELECT
    tenant.clusters.id,
    tenant.clusters.organization_id,
    tenant.clusters.name,
    tenant.clusters.region,
    tenant.clusters.kubernetes_version,
    tenant.clusters.created,
    tenant.clusters.deleted,
    tenant.clusters.shoot_status,
    tenant.clusters.shoot_status_message,
    tenant.clusters.shoot_status_updated,
    tenant.clusters.shoot_updating,
    tenant.cluster_sync_state.outbox_status,
    COALESCE(tenant.cluster_sync_state.outbox_retries, 0)::integer AS outbox_retries,
    tenant.cluster_sync_state.outbox_error,
    tenant.cluster_node_pool_sync_failure.node_pool_name AS failed_node_pool_name,
    tenant.cluster_node_pool_sync_failure.outbox_error AS failed_node_pool_error
FROM tenant.clusters
    LEFT JOIN tenant.cluster_sync_state ON tenant.cluster_sync_state.cluster_id = tenant.clusters.id
    LEFT JOIN tenant.cluster_node_pool_sync_failure ON tenant.cluster_node_pool_sync_failure.cluster_id = tenant.clusters.id
WHERE tenant.clusters.name = $1 AND tenant.clusters.deleted IS NULL;

-- name: ClusterCreate :one
-- Create a cluster if no active or pending-delete cluster with the same name exists.
-- Allows creation only after Gardener confirms deletion (shoot_status = 'deleted').
-- Returns NULL if blocked (caller should check for pgx.ErrNoRows).
-- The organization must be live. FOR SHARE waits for a funops organization
-- delete holding FOR UPDATE on the row; under read committed the deleted check
-- is then re-evaluated against the committed row, so the insert finds nothing
-- instead of creating a cluster nobody can manage.
-- region_id/kubernetes_version_id are the catalog references (expand phase: the
-- legacy text columns are written alongside them).
INSERT INTO tenant.clusters (organization_id, name, region, kubernetes_version, region_id, kubernetes_version_id)
SELECT $1, $2, $3, $4, sqlc.narg('region_id'), sqlc.narg('kubernetes_version_id')
WHERE EXISTS (
    SELECT 1
    FROM tenant.organizations
    WHERE id = $1
      AND deleted IS NULL
    FOR SHARE
)
AND NOT EXISTS (
    SELECT 1
    FROM tenant.clusters
    WHERE organization_id = $1
      AND name = $2
      AND (deleted IS NULL OR shoot_status IS DISTINCT FROM 'deleted')
)
RETURNING id;

-- name: ClusterUpdate :execrows
-- kubernetes_version_id is resolved server-side from the catalog together with
-- the version text (expand phase: both columns updated in lockstep).
UPDATE tenant.clusters
SET kubernetes_version = COALESCE(sqlc.narg('kubernetes_version'), kubernetes_version),
    kubernetes_version_id = COALESCE(sqlc.narg('kubernetes_version_id'), kubernetes_version_id)
WHERE id = $1 AND deleted IS NULL;

-- name: ClusterDelete :execrows
UPDATE tenant.clusters
SET deleted = NOW()
WHERE id = $1 AND deleted IS NULL;

-- name: ClusterGetEvents :many
-- Get event history for a cluster
SELECT
    id,
    cluster_id,
    event_type,
    created,
    sync_action,
    message,
    attempt
FROM tenant.cluster_events
WHERE cluster_id = $1
ORDER BY created DESC, id DESC
LIMIT $2;

-- name: ClusterCreateSyncRequestedEvent :exec
-- Insert sync_requested event when cluster is created/updated.
INSERT INTO tenant.cluster_events (
    cluster_id,
    event_type,
    sync_action
)
VALUES ($1, 'sync_requested', $2);

