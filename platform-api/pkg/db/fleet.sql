-- name: FleetTotals :one
-- Totals over live clusters: a cluster is live when it and its organization
-- are. The system organization is no tenant and is left out everywhere. Node,
-- pod and capacity sums count ready clusters only: the inventory collects
-- nothing else, so a cluster in error or deleting would otherwise go on
-- contributing its last numbers. stale_before is NULL while the inventory
-- loop has not run, and then nothing counts as stale.
SELECT
    (
        SELECT count(*)
        FROM tenant.organizations
        WHERE tenant.organizations.deleted IS NULL
            AND tenant.organizations.id <> tenant.system_organization_id()
    )::integer AS organizations,
    count(tenant.clusters.id)::integer AS clusters,
    coalesce(sum(tenant.cluster_snapshots.nodes) FILTER (WHERE tenant.clusters.shoot_status = 'ready'), 0)::integer AS nodes,
    coalesce(sum(tenant.cluster_snapshots.nodes_ready) FILTER (WHERE tenant.clusters.shoot_status = 'ready'), 0)::integer AS nodes_ready,
    coalesce(sum(tenant.cluster_snapshots.pods) FILTER (WHERE tenant.clusters.shoot_status = 'ready'), 0)::integer AS pods,
    coalesce(sum(tenant.cluster_snapshots.pod_allocatable) FILTER (WHERE tenant.clusters.shoot_status = 'ready'), 0)::integer AS pod_allocatable,
    coalesce(sum(tenant.cluster_snapshots.cpu_allocatable_m) FILTER (WHERE tenant.clusters.shoot_status = 'ready'), 0)::bigint AS cpu_allocatable_m,
    coalesce(sum(tenant.cluster_snapshots.memory_allocatable_mi) FILTER (WHERE tenant.clusters.shoot_status = 'ready'), 0)::bigint AS memory_allocatable_mi,
    count(tenant.clusters.id) FILTER (
        WHERE tenant.clusters.shoot_status = 'ready'
            AND coalesce(
                tenant.cluster_snapshots.observed,
                tenant.clusters.shoot_status_updated,
                tenant.clusters.created
            ) < sqlc.narg('stale_before')::timestamptz
    )::integer AS stale_snapshots,
    (
        SELECT count(*)
        FROM tenant.drift_findings
        LEFT JOIN tenant.clusters ON tenant.clusters.id = tenant.drift_findings.cluster_id
        WHERE tenant.drift_findings.resolved IS NULL
            AND tenant.clusters.organization_id IS DISTINCT FROM tenant.system_organization_id()
    )::integer AS drift_findings
FROM
    tenant.clusters
    JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
    LEFT JOIN tenant.cluster_snapshots ON tenant.cluster_snapshots.cluster_id = tenant.clusters.id
WHERE
    tenant.clusters.deleted IS NULL
    AND tenant.organizations.deleted IS NULL
    AND tenant.organizations.id <> tenant.system_organization_id();

-- name: WorkerHeartbeatList :many
-- When each of cluster-worker's periodic loops last completed a pass.
SELECT
    tenant.worker_heartbeats.worker,
    tenant.worker_heartbeats.interval_seconds,
    tenant.worker_heartbeats.last_pass
FROM
    tenant.worker_heartbeats
ORDER BY
    tenant.worker_heartbeats.worker;

-- name: ClusterStatusInputList :many
-- What every live cluster's status is derived from (common/clusterstatus),
-- optionally of some organizations only. The status is derived in Go, so the
-- unhealthy counts follow the same rule as the console.
SELECT
    tenant.clusters.id,
    tenant.clusters.organization_id,
    tenant.clusters.shoot_status,
    tenant.clusters.shoot_status_message,
    tenant.clusters.shoot_health,
    tenant.clusters.shoot_updating,
    tenant.cluster_sync_state.outbox_status,
    -- reached_gardener: a sync of the cluster ever completed, as in
    -- organization-api's ClusterList.
    EXISTS (
        SELECT 1
        FROM tenant.cluster_outbox
        WHERE tenant.cluster_outbox.cluster_id = tenant.clusters.id
            AND tenant.cluster_outbox.status = 'completed'
    )::boolean AS reached_gardener
FROM
    tenant.clusters
    JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
    LEFT JOIN tenant.cluster_sync_state ON tenant.cluster_sync_state.cluster_id = tenant.clusters.id
WHERE
    tenant.clusters.deleted IS NULL
    AND tenant.organizations.deleted IS NULL
    AND tenant.organizations.id <> tenant.system_organization_id()
    AND (
        sqlc.narg('organization_ids')::uuid[] IS NULL
        OR tenant.clusters.organization_id = ANY (sqlc.narg('organization_ids')::uuid[])
    );

-- name: OrganizationList :many
-- Live organizations, or the one asked for, with the aggregates of their live
-- clusters; node and pod sums of ready clusters only, as in FleetTotals. The
-- clusters and their snapshots are joined once and grouped per organization.
-- Paged by (alias, name, id) after the given key.
SELECT
    tenant.organizations.id,
    tenant.organizations.name,
    tenant.organizations.alias,
    tenant.organizations.created,
    count(tenant.clusters.id)::integer AS clusters,
    coalesce(sum(tenant.cluster_snapshots.nodes) FILTER (WHERE tenant.clusters.shoot_status = 'ready'), 0)::integer AS nodes,
    coalesce(sum(tenant.cluster_snapshots.nodes_ready) FILTER (WHERE tenant.clusters.shoot_status = 'ready'), 0)::integer AS nodes_ready,
    coalesce(sum(tenant.cluster_snapshots.pods) FILTER (WHERE tenant.clusters.shoot_status = 'ready'), 0)::integer AS pods,
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
    LEFT JOIN tenant.clusters ON tenant.clusters.organization_id = tenant.organizations.id
        AND tenant.clusters.deleted IS NULL
    LEFT JOIN tenant.cluster_snapshots ON tenant.cluster_snapshots.cluster_id = tenant.clusters.id
WHERE
    tenant.organizations.deleted IS NULL
    AND tenant.organizations.id <> tenant.system_organization_id()
    AND (sqlc.narg('organization_id')::uuid IS NULL OR tenant.organizations.id = sqlc.narg('organization_id'))
    AND (
        sqlc.narg('after_id')::uuid IS NULL
        OR (tenant.organizations.alias, tenant.organizations.name, tenant.organizations.id)
            > (sqlc.narg('after_alias')::text, sqlc.narg('after_name')::text, sqlc.narg('after_id')::uuid)
    )
GROUP BY
    organizations.id
ORDER BY
    tenant.organizations.alias,
    tenant.organizations.name,
    tenant.organizations.id
LIMIT @page_limit::integer;

-- name: ClusterList :many
-- Live clusters, optionally of one organization or just one cluster, with
-- their latest snapshot and what their status is derived from. stale: a ready
-- cluster whose snapshot, or readiness when it has none, is older than
-- stale_before; never while stale_before is NULL. only_stale keeps the stale
-- ones, by the same rule, so the STALE filter does not page through the whole
-- fleet. Paged by (organization alias, cluster name, cluster id) after the
-- given key.
SELECT
    tenant.clusters.id,
    tenant.clusters.organization_id,
    tenant.organizations.name AS organization_name,
    tenant.organizations.alias AS organization_alias,
    tenant.clusters.name,
    tenant.clusters.region,
    tenant.clusters.kubernetes_version,
    tenant.clusters.shoot_status,
    tenant.clusters.shoot_status_message,
    tenant.clusters.shoot_health,
    tenant.clusters.shoot_updating,
    tenant.cluster_sync_state.outbox_status,
    EXISTS (
        SELECT 1
        FROM tenant.cluster_outbox
        WHERE tenant.cluster_outbox.cluster_id = tenant.clusters.id
            AND tenant.cluster_outbox.status = 'completed'
    )::boolean AS reached_gardener,
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
    coalesce(
        tenant.clusters.shoot_status = 'ready'
        AND coalesce(
            tenant.cluster_snapshots.observed,
            tenant.clusters.shoot_status_updated,
            tenant.clusters.created
        ) < sqlc.narg('stale_before')::timestamptz,
        false
    )::boolean AS stale
FROM
    tenant.clusters
    JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
    LEFT JOIN tenant.cluster_snapshots ON tenant.cluster_snapshots.cluster_id = tenant.clusters.id
    LEFT JOIN tenant.cluster_sync_state ON tenant.cluster_sync_state.cluster_id = tenant.clusters.id
WHERE
    tenant.clusters.deleted IS NULL
    AND tenant.organizations.deleted IS NULL
    AND tenant.organizations.id <> tenant.system_organization_id()
    AND (sqlc.narg('organization_id')::uuid IS NULL OR tenant.clusters.organization_id = sqlc.narg('organization_id'))
    AND (sqlc.narg('cluster_id')::uuid IS NULL OR tenant.clusters.id = sqlc.narg('cluster_id'))
    AND (
        NOT @only_stale::boolean
        OR (
            tenant.clusters.shoot_status = 'ready'
            AND coalesce(
                tenant.cluster_snapshots.observed,
                tenant.clusters.shoot_status_updated,
                tenant.clusters.created
            ) < sqlc.narg('stale_before')::timestamptz
        )
    )
    AND (
        sqlc.narg('after_id')::uuid IS NULL
        OR (tenant.organizations.alias, tenant.clusters.name, tenant.clusters.id)
            > (sqlc.narg('after_alias')::text, sqlc.narg('after_name')::text, sqlc.narg('after_id')::uuid)
    )
ORDER BY
    tenant.organizations.alias,
    tenant.clusters.name,
    tenant.clusters.id
LIMIT @page_limit::integer;

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
