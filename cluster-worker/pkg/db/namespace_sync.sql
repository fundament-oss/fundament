-- name: NamespaceGetForSync :one
-- Resolve a namespace to its owning cluster and organization for sync.
-- Joins namespaces -> projects -> clusters. The organization is derived from
-- the cluster (Organization -> Cluster -> Project -> Namespace per ADR-0011,
-- strictly one cluster per project). shoot_status is returned so the handler
-- can defer (PreconditionError) while the shoot is not yet ready. The row is
-- returned regardless of cluster/shoot readiness so the handler can decide.
-- project_name leads the deterministic cluster-side namespace name so namespaces
-- from different projects on the same shoot never collide.
-- The cluster_default_*/project_default_* columns are the per-container
-- resource defaults of the owning cluster and project; NULL means no default
-- is set for that field. The handler takes the project's value where it has
-- one and the cluster's otherwise when building the namespace's LimitRange.
-- That merge lives in Go rather than a SQL COALESCE() because sqlc cannot
-- infer a nullable type for computed columns.
SELECT
    tenant.namespaces.id,
    tenant.namespaces.project_id,
    tenant.projects.cluster_id,
    tenant.projects.name AS project_name,
    tenant.clusters.organization_id,
    tenant.namespaces.name,
    tenant.namespaces.deleted,
    tenant.clusters.shoot_status,
    tenant.clusters.shoot_api_available,
    tenant.clusters.default_cpu_request_m AS cluster_default_cpu_request_m,
    tenant.clusters.default_cpu_limit_m AS cluster_default_cpu_limit_m,
    tenant.clusters.default_memory_request_mi AS cluster_default_memory_request_mi,
    tenant.clusters.default_memory_limit_mi AS cluster_default_memory_limit_mi,
    tenant.projects.default_cpu_request_m AS project_default_cpu_request_m,
    tenant.projects.default_cpu_limit_m AS project_default_cpu_limit_m,
    tenant.projects.default_memory_request_mi AS project_default_memory_request_mi,
    tenant.projects.default_memory_limit_mi AS project_default_memory_limit_mi
FROM tenant.namespaces
JOIN tenant.projects ON tenant.projects.id = tenant.namespaces.project_id
JOIN tenant.clusters ON tenant.clusters.id = tenant.projects.cluster_id
WHERE tenant.namespaces.id = @id;

-- name: NamespaceListActiveForCluster :many
-- List the ids of active (deleted IS NULL) namespaces owned by a cluster.
-- Used by the api_available fan-out and by reconcile: this is the desired
-- set of cluster-side namespaces. A cluster-side namespace whose id is not in
-- this set is an orphan (DB row missing or soft-deleted).
SELECT tenant.namespaces.id
FROM tenant.namespaces
JOIN tenant.projects ON tenant.projects.id = tenant.namespaces.project_id
WHERE tenant.projects.cluster_id = @cluster_id
  AND tenant.namespaces.deleted IS NULL;

-- name: NamespaceListFailedForCluster :many
-- List the ids of a cluster's active namespaces whose latest sync row failed
-- for good and whose failure is older than the retry interval. The reconcile
-- loop queues these again, whatever the Shoot already shows: a sync can fail
-- after creating the namespace, converging its LimitRange or role bindings,
-- and the namespace then looks present while staying unusable. A soft-deleted
-- namespace is left out on purpose: a failed delete row leaves the object on
-- the Shoot, where the reconcile loop's orphan sweep removes it.
SELECT tenant.namespaces.id
FROM tenant.namespaces
JOIN tenant.projects ON tenant.projects.id = tenant.namespaces.project_id
WHERE tenant.projects.cluster_id = @cluster_id
  AND tenant.namespaces.deleted IS NULL
  AND COALESCE((
      SELECT
          tenant.cluster_outbox.status = 'failed'
          AND tenant.cluster_outbox.failed <= now() - @failed_retry_after::interval
      FROM tenant.cluster_outbox
      WHERE tenant.cluster_outbox.namespace_id = tenant.namespaces.id
      ORDER BY tenant.cluster_outbox.id DESC
      LIMIT 1
  ), false);
