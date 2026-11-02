-- name: InventoryClusterList :many
-- Clusters the inventory worker collects: live, of a live organization, and
-- ready. Before ready there is no shoot API to read; a deleted cluster keeps
-- its last snapshot.
SELECT
    tenant.clusters.id
FROM
    tenant.clusters
    JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
WHERE
    tenant.clusters.deleted IS NULL
    AND tenant.organizations.deleted IS NULL
    AND tenant.clusters.shoot_status = 'ready'
ORDER BY
    tenant.clusters.id;

-- name: InventoryNodePoolList :many
-- Live node pools of a cluster, to match against the shoot's worker pools.
SELECT
    tenant.node_pools.id,
    tenant.node_pools.name
FROM
    tenant.node_pools
WHERE
    tenant.node_pools.cluster_id = @cluster_id
    AND tenant.node_pools.deleted IS NULL
ORDER BY
    tenant.node_pools.id;

-- name: InventoryNamespaceList :many
-- Live namespaces of a cluster. A namespace the shoot carries a label for but
-- the database no longer lists is not snapshotted.
SELECT
    tenant.namespaces.id
FROM
    tenant.namespaces
    JOIN tenant.projects ON tenant.projects.id = tenant.namespaces.project_id
WHERE
    tenant.projects.cluster_id = @cluster_id
    AND tenant.projects.deleted IS NULL
    AND tenant.namespaces.deleted IS NULL
ORDER BY
    tenant.namespaces.id;

-- name: ClusterSnapshotUpsert :exec
-- Record a successful collection; it clears the previous error.
INSERT INTO
    tenant.cluster_snapshots (
        cluster_id,
        observed,
        nodes,
        nodes_ready,
        pods,
        pod_allocatable,
        cpu_allocatable_m,
        memory_allocatable_mi
    )
VALUES
    (
        @cluster_id,
        @observed,
        @nodes,
        @nodes_ready,
        @pods,
        @pod_allocatable,
        @cpu_allocatable_m,
        @memory_allocatable_mi
    )
ON CONFLICT (cluster_id) DO UPDATE
SET
    observed = EXCLUDED.observed,
    nodes = EXCLUDED.nodes,
    nodes_ready = EXCLUDED.nodes_ready,
    pods = EXCLUDED.pods,
    pod_allocatable = EXCLUDED.pod_allocatable,
    cpu_allocatable_m = EXCLUDED.cpu_allocatable_m,
    memory_allocatable_mi = EXCLUDED.memory_allocatable_mi,
    last_error = NULL,
    last_error_at = NULL;

-- name: ClusterSnapshotRecordError :exec
-- Record a failed collection. The last good numbers and their observed time
-- stay, so the staff page shows how old they are next to the error.
INSERT INTO
    tenant.cluster_snapshots (
        cluster_id,
        last_error,
        last_error_at
    )
VALUES
    (
        @cluster_id,
        @last_error,
        @last_error_at
    )
ON CONFLICT (cluster_id) DO UPDATE
SET
    last_error = EXCLUDED.last_error,
    last_error_at = EXCLUDED.last_error_at;

-- name: NodePoolSnapshotUpsert :exec
-- One row per node pool, the arrays zipped by position.
INSERT INTO
    tenant.node_pool_snapshots (
        node_pool_id,
        observed,
        nodes,
        nodes_ready
    )
SELECT
    unnest(@node_pool_ids::uuid[]),
    @observed::timestamptz,
    unnest(@nodes::integer[]),
    unnest(@nodes_ready::integer[])
ON CONFLICT (node_pool_id) DO UPDATE
SET
    observed = EXCLUDED.observed,
    nodes = EXCLUDED.nodes,
    nodes_ready = EXCLUDED.nodes_ready;

-- name: NamespaceSnapshotUpsert :exec
-- One row per namespace, the arrays zipped by position.
INSERT INTO
    tenant.namespace_snapshots (
        namespace_id,
        observed,
        pods
    )
SELECT
    unnest(@namespace_ids::uuid[]),
    @observed::timestamptz,
    unnest(@pods::integer[])
ON CONFLICT (namespace_id) DO UPDATE
SET
    observed = EXCLUDED.observed,
    pods = EXCLUDED.pods;

-- name: InventoryMockNodePoolList :many
-- Live node pools of a cluster with their autoscaler minimum, which the mock
-- inventory runs as nodes (GARDENER_MODE=mock).
SELECT
    tenant.node_pools.name,
    tenant.node_pools.autoscale_min
FROM
    tenant.node_pools
WHERE
    tenant.node_pools.cluster_id = @cluster_id
    AND tenant.node_pools.deleted IS NULL
ORDER BY
    tenant.node_pools.name;

-- name: InventoryMockNamespaceList :many
-- Live namespaces of a cluster by name, for the mock inventory's pod counts.
SELECT
    tenant.namespaces.id,
    tenant.namespaces.name
FROM
    tenant.namespaces
    JOIN tenant.projects ON tenant.projects.id = tenant.namespaces.project_id
WHERE
    tenant.projects.cluster_id = @cluster_id
    AND tenant.projects.deleted IS NULL
    AND tenant.namespaces.deleted IS NULL
ORDER BY
    tenant.namespaces.id;
