-- name: NamespaceGetForSync :one
-- Resolve a namespace to its owning cluster and organization for sync.
-- Joins namespaces -> projects -> clusters. The organization is derived from
-- the cluster (Organization -> Cluster -> Project -> Namespace per ADR-0011,
-- strictly one cluster per project). shoot_status is returned so the handler
-- can defer (PreconditionError) while the shoot is not yet ready. The row is
-- returned regardless of cluster/shoot readiness so the handler can decide.
SELECT
    tenant.namespaces.id,
    tenant.namespaces.project_id,
    tenant.projects.cluster_id,
    tenant.clusters.organization_id,
    tenant.namespaces.name,
    tenant.namespaces.deleted,
    tenant.clusters.shoot_status
FROM tenant.namespaces
JOIN tenant.projects ON tenant.projects.id = tenant.namespaces.project_id
JOIN tenant.clusters ON tenant.clusters.id = tenant.projects.cluster_id
WHERE tenant.namespaces.id = @id;

-- name: NamespaceListActiveForCluster :many
-- List the ids of active (deleted IS NULL) namespaces owned by a cluster.
-- Used by the cluster-ready fan-out and by reconcile: this is the desired
-- set of cluster-side namespaces. A cluster-side namespace whose id is not in
-- this set is an orphan (DB row missing or soft-deleted).
SELECT tenant.namespaces.id
FROM tenant.namespaces
JOIN tenant.projects ON tenant.projects.id = tenant.namespaces.project_id
WHERE tenant.projects.cluster_id = @cluster_id
  AND tenant.namespaces.deleted IS NULL;
