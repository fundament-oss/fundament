-- Queries used by the cluster handler's Reconcile method.

-- name: ClusterListActive :many
-- Used by periodic reconciliation to compare with Gardener state.
SELECT
    tenant.clusters.id,
    tenant.clusters.name,
    tenant.clusters.deleted,
    tenant.clusters.synced,
    tenant.organizations.name AS organization_name
FROM
    tenant.clusters
    JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
WHERE
    tenant.clusters.deleted IS NULL;
