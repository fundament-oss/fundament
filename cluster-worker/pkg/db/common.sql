-- name: ClusterGetForSync :one
-- Get cluster with the fields needed to build a gardener.ClusterToSync.
-- Used by the cluster handler's Sync() method.
SELECT
    tenant.clusters.id,
    tenant.clusters.name,
    tenant.clusters.region,
    tenant.clusters.kubernetes_version,
    tenant.clusters.deleted,
    tenant.clusters.organization_id,
    tenant.organizations.name AS organization_name
FROM
    tenant.clusters
    JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
WHERE
    tenant.clusters.id = @cluster_id;
