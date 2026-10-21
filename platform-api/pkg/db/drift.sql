-- name: DriftShootWithoutCluster :many
-- A live shoot whose cluster id matches no cluster at all.
SELECT
    tenant.gardener_shoots.cluster_id,
    tenant.gardener_shoots.shoot_name,
    tenant.gardener_shoots.shoot_namespace,
    tenant.gardener_shoots.first_seen
FROM
    tenant.gardener_shoots
WHERE
    tenant.gardener_shoots.deleted IS NULL
    AND NOT EXISTS (
        SELECT 1
        FROM tenant.clusters
        WHERE tenant.clusters.id = tenant.gardener_shoots.cluster_id
    )
ORDER BY
    tenant.gardener_shoots.first_seen;

-- name: DriftShootOfDeletedCluster :many
-- A live shoot of a cluster that is deleted, or whose organization is. The
-- disagreement started when the cluster was deleted.
SELECT
    tenant.gardener_shoots.cluster_id,
    tenant.gardener_shoots.shoot_name,
    tenant.gardener_shoots.shoot_namespace,
    tenant.clusters.name AS cluster_name,
    tenant.clusters.organization_id,
    tenant.organizations.name AS organization_name,
    coalesce(tenant.clusters.deleted, tenant.organizations.deleted)::timestamptz AS since
FROM
    tenant.gardener_shoots
    JOIN tenant.clusters ON tenant.clusters.id = tenant.gardener_shoots.cluster_id
    JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
WHERE
    tenant.gardener_shoots.deleted IS NULL
    AND (tenant.clusters.deleted IS NOT NULL OR tenant.organizations.deleted IS NOT NULL)
ORDER BY
    since;

-- name: DriftClusterWithoutShoot :many
-- A live cluster that reached Gardener but has no live shoot. Reported only
-- while the inventory was refreshed after fresh_after, and only for clusters
-- whose last status change predates that refresh: a stopped worker, or a
-- cluster created since the last reconcile, is not drift. since is when its
-- shoot was last seen to go, or else when it was synced.
SELECT
    tenant.clusters.id AS cluster_id,
    tenant.clusters.name AS cluster_name,
    tenant.clusters.organization_id,
    tenant.organizations.name AS organization_name,
    coalesce(
        (
            SELECT max(tenant.gardener_shoots.deleted)
            FROM tenant.gardener_shoots
            WHERE tenant.gardener_shoots.cluster_id = tenant.clusters.id
        ),
        tenant.clusters.shoot_status_updated,
        tenant.clusters.created
    )::timestamptz AS since
FROM
    tenant.clusters
    JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
    JOIN tenant.cluster_sync_state ON tenant.cluster_sync_state.cluster_id = tenant.clusters.id
WHERE
    tenant.clusters.deleted IS NULL
    AND tenant.organizations.deleted IS NULL
    AND tenant.cluster_sync_state.outbox_status = 'completed'
    AND NOT EXISTS (
        SELECT 1
        FROM tenant.gardener_shoots
        WHERE tenant.gardener_shoots.cluster_id = tenant.clusters.id
            AND tenant.gardener_shoots.deleted IS NULL
    )
    AND coalesce(tenant.clusters.shoot_status_updated, tenant.clusters.created) < (
        SELECT max(tenant.gardener_shoots.last_seen)
        FROM tenant.gardener_shoots
        WHERE tenant.gardener_shoots.deleted IS NULL
    )
    AND (
        SELECT max(tenant.gardener_shoots.last_seen)
        FROM tenant.gardener_shoots
        WHERE tenant.gardener_shoots.deleted IS NULL
    ) >= @fresh_after::timestamptz
ORDER BY
    since;

-- name: DriftSnapshotStale :many
-- A ready live cluster with no snapshot, or one older than stale_before. A
-- cluster that became ready less than stale_before ago is given time for its
-- first collection. Same rule as ClusterList's stale column.
SELECT
    tenant.clusters.id AS cluster_id,
    tenant.clusters.name AS cluster_name,
    tenant.clusters.organization_id,
    tenant.organizations.name AS organization_name,
    tenant.cluster_snapshots.observed,
    tenant.cluster_snapshots.last_error,
    coalesce(
        tenant.cluster_snapshots.observed,
        tenant.clusters.shoot_status_updated,
        tenant.clusters.created
    )::timestamptz AS since
FROM
    tenant.clusters
    JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
    LEFT JOIN tenant.cluster_snapshots ON tenant.cluster_snapshots.cluster_id = tenant.clusters.id
WHERE
    tenant.clusters.deleted IS NULL
    AND tenant.organizations.deleted IS NULL
    AND tenant.clusters.shoot_status = 'ready'
    AND coalesce(
        tenant.cluster_snapshots.observed,
        tenant.clusters.shoot_status_updated,
        tenant.clusters.created
    ) < @stale_before::timestamptz
ORDER BY
    since;

-- name: DriftKubernetesVersionMismatch :many
-- A ready live cluster whose shoot runs another Kubernetes version while no
-- update is rolling out. The shoot must have been seen after the cluster's
-- last status change, so a version applied since the last reconcile is not
-- reported against the shoot's older spec.
SELECT
    tenant.clusters.id AS cluster_id,
    tenant.clusters.name AS cluster_name,
    tenant.clusters.organization_id,
    tenant.organizations.name AS organization_name,
    tenant.clusters.kubernetes_version,
    tenant.gardener_shoots.kubernetes_version AS shoot_kubernetes_version,
    tenant.gardener_shoots.shoot_name,
    tenant.gardener_shoots.shoot_namespace,
    tenant.clusters.shoot_status_updated AS since
FROM
    tenant.clusters
    JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
    JOIN tenant.gardener_shoots ON tenant.gardener_shoots.cluster_id = tenant.clusters.id
        AND tenant.gardener_shoots.deleted IS NULL
WHERE
    tenant.clusters.deleted IS NULL
    AND tenant.organizations.deleted IS NULL
    AND tenant.clusters.shoot_status = 'ready'
    AND NOT tenant.clusters.shoot_updating
    AND tenant.clusters.kubernetes_version <> tenant.gardener_shoots.kubernetes_version
    AND tenant.gardener_shoots.last_seen > coalesce(tenant.clusters.shoot_status_updated, tenant.clusters.created)
ORDER BY
    tenant.clusters.name;
