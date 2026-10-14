-- name: FleetTotals :one
-- Totals over live clusters: a cluster is live when it and its organization
-- are. The seeded system organization owns first-party plugins and is no
-- tenant, so it is left out here as on the public page.
SELECT
    (
        SELECT count(*)
        FROM tenant.organizations
        WHERE tenant.organizations.deleted IS NULL
            AND tenant.organizations.id <> '019b4000-0000-7000-8000-000000000000'::uuid
    )::integer AS organizations,
    count(tenant.clusters.id)::integer AS clusters,
    count(tenant.clusters.id) FILTER (
        WHERE tenant.clusters.shoot_health = 'unhealthy' OR tenant.clusters.shoot_status = 'error'
    )::integer AS clusters_unhealthy,
    count(tenant.clusters.id) FILTER (WHERE tenant.clusters.shoot_updating)::integer AS clusters_updating,
    coalesce(sum(tenant.cluster_snapshots.nodes), 0)::integer AS nodes,
    coalesce(sum(tenant.cluster_snapshots.nodes_ready), 0)::integer AS nodes_ready,
    coalesce(sum(tenant.cluster_snapshots.pods), 0)::integer AS pods,
    coalesce(sum(tenant.cluster_snapshots.pod_allocatable), 0)::integer AS pod_allocatable,
    coalesce(sum(tenant.cluster_snapshots.cpu_allocatable_m), 0)::bigint AS cpu_allocatable_m,
    coalesce(sum(tenant.cluster_snapshots.memory_allocatable_mi), 0)::bigint AS memory_allocatable_mi,
    (
        SELECT max(tenant.gardener_shoots.last_seen)
        FROM tenant.gardener_shoots
        WHERE tenant.gardener_shoots.deleted IS NULL
    )::timestamptz AS gardener_inventory_refreshed
FROM
    tenant.clusters
    JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
    LEFT JOIN tenant.cluster_snapshots ON tenant.cluster_snapshots.cluster_id = tenant.clusters.id
WHERE
    tenant.clusters.deleted IS NULL
    AND tenant.organizations.deleted IS NULL
    AND tenant.organizations.id <> '019b4000-0000-7000-8000-000000000000'::uuid;

-- name: OrganizationList :many
-- Every live organization, or the one asked for, with its limits and the
-- aggregates of its live clusters.
SELECT
    tenant.organizations.id,
    tenant.organizations.name,
    tenant.organizations.alias,
    tenant.organizations.created,
    tenant.organization_limits.max_nodes_per_cluster,
    tenant.organization_limits.max_node_pools_per_cluster,
    tenant.organization_limits.max_nodes_per_node_pool,
    (
        SELECT count(*)
        FROM tenant.clusters
        WHERE tenant.clusters.organization_id = tenant.organizations.id
            AND tenant.clusters.deleted IS NULL
    )::integer AS clusters,
    (
        SELECT count(*)
        FROM tenant.clusters
        WHERE tenant.clusters.organization_id = tenant.organizations.id
            AND tenant.clusters.deleted IS NULL
            AND (tenant.clusters.shoot_health = 'unhealthy' OR tenant.clusters.shoot_status = 'error')
    )::integer AS clusters_unhealthy,
    (
        SELECT coalesce(sum(tenant.cluster_snapshots.nodes), 0)
        FROM tenant.cluster_snapshots
        JOIN tenant.clusters ON tenant.clusters.id = tenant.cluster_snapshots.cluster_id
        WHERE tenant.clusters.organization_id = tenant.organizations.id
            AND tenant.clusters.deleted IS NULL
    )::integer AS nodes,
    (
        SELECT coalesce(sum(tenant.cluster_snapshots.nodes_ready), 0)
        FROM tenant.cluster_snapshots
        JOIN tenant.clusters ON tenant.clusters.id = tenant.cluster_snapshots.cluster_id
        WHERE tenant.clusters.organization_id = tenant.organizations.id
            AND tenant.clusters.deleted IS NULL
    )::integer AS nodes_ready,
    (
        SELECT coalesce(sum(tenant.cluster_snapshots.pods), 0)
        FROM tenant.cluster_snapshots
        JOIN tenant.clusters ON tenant.clusters.id = tenant.cluster_snapshots.cluster_id
        WHERE tenant.clusters.organization_id = tenant.organizations.id
            AND tenant.clusters.deleted IS NULL
    )::integer AS pods,
    (
        SELECT count(*)
        FROM tenant.namespaces
        JOIN tenant.projects ON tenant.projects.id = tenant.namespaces.project_id
        JOIN tenant.clusters ON tenant.clusters.id = tenant.projects.cluster_id
        WHERE tenant.clusters.organization_id = tenant.organizations.id
            AND tenant.clusters.deleted IS NULL
            AND tenant.projects.deleted IS NULL
            AND tenant.namespaces.deleted IS NULL
    )::integer AS namespaces
FROM
    tenant.organizations
    LEFT JOIN tenant.organization_limits ON tenant.organization_limits.organization_id = tenant.organizations.id
        AND tenant.organization_limits.deleted IS NULL
WHERE
    tenant.organizations.deleted IS NULL
    AND tenant.organizations.id <> '019b4000-0000-7000-8000-000000000000'::uuid
    AND (sqlc.narg('organization_id')::uuid IS NULL OR tenant.organizations.id = sqlc.narg('organization_id'))
ORDER BY
    tenant.organizations.alias,
    tenant.organizations.name;

-- name: ClusterList :many
-- Live clusters, optionally of one organization or just one cluster, with
-- their latest snapshot and the organization's limits. node_pool_at_limit is
-- whether one of its pools was last seen at the per-pool cap. stale is computed here
-- with the same rule as the SNAPSHOT_STALE drift query: a ready cluster whose
-- snapshot, or readiness when it has none, is older than stale_before.
SELECT
    tenant.clusters.id,
    tenant.clusters.organization_id,
    tenant.organizations.name AS organization_name,
    tenant.clusters.name,
    tenant.clusters.region,
    tenant.clusters.kubernetes_version,
    tenant.clusters.shoot_status,
    tenant.clusters.shoot_status_message,
    tenant.clusters.shoot_health,
    tenant.clusters.shoot_updating,
    tenant.clusters.created,
    (
        SELECT count(*)
        FROM tenant.node_pools
        WHERE tenant.node_pools.cluster_id = tenant.clusters.id
            AND tenant.node_pools.deleted IS NULL
    )::integer AS node_pools,
    (
        SELECT coalesce(sum(tenant.node_pools.autoscale_min), 0)
        FROM tenant.node_pools
        WHERE tenant.node_pools.cluster_id = tenant.clusters.id
            AND tenant.node_pools.deleted IS NULL
    )::integer AS nodes_min,
    (
        SELECT coalesce(sum(tenant.node_pools.autoscale_max), 0)
        FROM tenant.node_pools
        WHERE tenant.node_pools.cluster_id = tenant.clusters.id
            AND tenant.node_pools.deleted IS NULL
    )::integer AS nodes_max,
    (tenant.cluster_snapshots.id IS NOT NULL)::boolean AS has_snapshot,
    tenant.cluster_snapshots.observed,
    tenant.cluster_snapshots.nodes,
    tenant.cluster_snapshots.nodes_ready,
    tenant.cluster_snapshots.pods,
    tenant.cluster_snapshots.pod_allocatable,
    tenant.cluster_snapshots.cpu_allocatable_m,
    tenant.cluster_snapshots.memory_allocatable_mi,
    tenant.cluster_snapshots.last_error,
    tenant.cluster_snapshots.last_error_at,
    tenant.organization_limits.max_nodes_per_cluster,
    tenant.organization_limits.max_node_pools_per_cluster,
    tenant.organization_limits.max_nodes_per_node_pool,
    (
        tenant.organization_limits.max_nodes_per_node_pool IS NOT NULL
        AND EXISTS (
            SELECT 1
            FROM tenant.node_pools
            JOIN tenant.node_pool_snapshots ON tenant.node_pool_snapshots.node_pool_id = tenant.node_pools.id
            WHERE tenant.node_pools.cluster_id = tenant.clusters.id
                AND tenant.node_pools.deleted IS NULL
                AND tenant.node_pool_snapshots.nodes >= tenant.organization_limits.max_nodes_per_node_pool
        )
    )::boolean AS node_pool_at_limit,
    (
        tenant.clusters.shoot_status = 'ready'
        AND coalesce(
            tenant.cluster_snapshots.observed,
            tenant.clusters.shoot_status_updated,
            tenant.clusters.created
        ) < @stale_before::timestamptz
    )::boolean AS stale
FROM
    tenant.clusters
    JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
    LEFT JOIN tenant.cluster_snapshots ON tenant.cluster_snapshots.cluster_id = tenant.clusters.id
    LEFT JOIN tenant.organization_limits ON tenant.organization_limits.organization_id = tenant.clusters.organization_id
        AND tenant.organization_limits.deleted IS NULL
WHERE
    tenant.clusters.deleted IS NULL
    AND tenant.organizations.deleted IS NULL
    AND (sqlc.narg('organization_id')::uuid IS NULL OR tenant.clusters.organization_id = sqlc.narg('organization_id'))
    AND (sqlc.narg('cluster_id')::uuid IS NULL OR tenant.clusters.id = sqlc.narg('cluster_id'))
ORDER BY
    tenant.organizations.alias,
    tenant.clusters.name;

-- name: NodePoolListByCluster :many
SELECT
    tenant.node_pools.id,
    tenant.node_pools.name,
    tenant.node_pools.machine_type,
    tenant.node_pools.autoscale_min,
    tenant.node_pools.autoscale_max,
    tenant.node_pool_snapshots.observed,
    tenant.node_pool_snapshots.nodes,
    tenant.node_pool_snapshots.nodes_ready
FROM
    tenant.node_pools
    LEFT JOIN tenant.node_pool_snapshots ON tenant.node_pool_snapshots.node_pool_id = tenant.node_pools.id
WHERE
    tenant.node_pools.cluster_id = @cluster_id
    AND tenant.node_pools.deleted IS NULL
ORDER BY
    tenant.node_pools.name;

-- name: ProjectListByCluster :many
SELECT
    tenant.projects.id,
    tenant.projects.name,
    tenant.projects.alias
FROM
    tenant.projects
WHERE
    tenant.projects.cluster_id = @cluster_id
    AND tenant.projects.deleted IS NULL
ORDER BY
    tenant.projects.name;

-- name: NamespaceListByCluster :many
SELECT
    tenant.namespaces.id,
    tenant.namespaces.project_id,
    tenant.namespaces.name,
    tenant.namespace_snapshots.observed,
    tenant.namespace_snapshots.pods
FROM
    tenant.namespaces
    JOIN tenant.projects ON tenant.projects.id = tenant.namespaces.project_id
    LEFT JOIN tenant.namespace_snapshots ON tenant.namespace_snapshots.namespace_id = tenant.namespaces.id
WHERE
    tenant.projects.cluster_id = @cluster_id
    AND tenant.projects.deleted IS NULL
    AND tenant.namespaces.deleted IS NULL
ORDER BY
    tenant.namespaces.name;

-- name: GardenerShootGetByCluster :one
-- The live shoot cluster-worker last saw for the cluster. Gardener has one
-- per cluster id; should a recreated one overlap, the newest wins.
SELECT
    tenant.gardener_shoots.shoot_name,
    tenant.gardener_shoots.shoot_namespace,
    tenant.gardener_shoots.kubernetes_version,
    tenant.gardener_shoots.region,
    tenant.gardener_shoots.shoot_created,
    tenant.gardener_shoots.last_seen
FROM
    tenant.gardener_shoots
WHERE
    tenant.gardener_shoots.cluster_id = @cluster_id
    AND tenant.gardener_shoots.deleted IS NULL
ORDER BY
    tenant.gardener_shoots.shoot_created DESC
LIMIT 1;
