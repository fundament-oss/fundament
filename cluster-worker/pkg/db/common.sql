-- name: ClusterGetEvents :many
-- Get event history for a cluster.
SELECT
    id,
    cluster_id,
    event_type,
    created,
    sync_action,
    message,
    attempt
FROM
    tenant.cluster_events
WHERE
    cluster_id = @cluster_id
ORDER BY
    created DESC
LIMIT
    @limit_count;

-- name: ClusterGetByID :one
-- Get a single cluster by ID with sync and status state (for testing).
SELECT
    tenant.clusters.id,
    tenant.clusters.name,
    tenant.clusters.deleted,
    tenant.clusters.synced,
    tenant.clusters.shoot_status,
    tenant.clusters.shoot_status_message,
    tenant.clusters.shoot_status_updated,
    tenant.organizations.name AS organization_name
FROM
    tenant.clusters
    JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
WHERE
    tenant.clusters.id = @cluster_id;
