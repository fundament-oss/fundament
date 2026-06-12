-- name: NamespaceGetForSync :one
-- Resolve a namespace to its owning cluster and organization for sync.
-- Joins namespaces -> projects -> clusters. The organization is derived from
-- the cluster (Organization -> Cluster -> Project -> Namespace per ADR-0011,
-- strictly one cluster per project). shoot_status is returned so the handler
-- can defer (PreconditionError) while the shoot is not yet ready. The row is
-- returned regardless of cluster/shoot readiness so the handler can decide.
-- project_name leads the deterministic cluster-side namespace name so namespaces
-- from different projects on the same shoot never collide.
-- The org_default_*/project_default_* columns are the active organization and
-- project per-container resource defaults (LEFT JOIN, so NULL when no active
-- limits row exists). The handler merges them per field (lowest non-NULL wins,
-- so a project default can only tighten the organization default) when
-- building the namespace's LimitRange. The merge lives in Go rather than a SQL
-- LEAST() because sqlc cannot infer a nullable type for computed columns.
SELECT
    tenant.namespaces.id,
    tenant.namespaces.project_id,
    tenant.projects.cluster_id,
    tenant.projects.name AS project_name,
    tenant.clusters.organization_id,
    tenant.namespaces.name,
    tenant.namespaces.deleted,
    tenant.clusters.shoot_status,
    tenant.organization_limits.default_cpu_request_m AS org_default_cpu_request_m,
    tenant.organization_limits.default_cpu_limit_m AS org_default_cpu_limit_m,
    tenant.organization_limits.default_memory_request_mi AS org_default_memory_request_mi,
    tenant.organization_limits.default_memory_limit_mi AS org_default_memory_limit_mi,
    tenant.project_limits.default_cpu_request_m AS project_default_cpu_request_m,
    tenant.project_limits.default_cpu_limit_m AS project_default_cpu_limit_m,
    tenant.project_limits.default_memory_request_mi AS project_default_memory_request_mi,
    tenant.project_limits.default_memory_limit_mi AS project_default_memory_limit_mi
FROM tenant.namespaces
JOIN tenant.projects ON tenant.projects.id = tenant.namespaces.project_id
JOIN tenant.clusters ON tenant.clusters.id = tenant.projects.cluster_id
LEFT JOIN tenant.project_limits ON tenant.project_limits.project_id = tenant.namespaces.project_id
    AND tenant.project_limits.deleted IS NULL
LEFT JOIN tenant.organization_limits ON tenant.organization_limits.organization_id = tenant.clusters.organization_id
    AND tenant.organization_limits.deleted IS NULL
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
