-- name: ClusterGetForSync :one
-- Get cluster with the fields needed to build a gardener.ClusterToSync.
-- Used by the cluster handler's Sync() method.
-- The catalog join resolves the region's cloud profile; NULL (legacy cluster
-- without region_id) falls back to the worker's provider defaults.
SELECT
    tenant.clusters.id,
    tenant.clusters.name,
    tenant.clusters.region,
    tenant.clusters.kubernetes_version,
    tenant.clusters.deleted,
    tenant.clusters.organization_id,
    tenant.organizations.name AS organization_name,
    catalog.regions.cloud_profile,
    catalog.regions.cloud_profile_region
FROM
    tenant.clusters
    JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
    LEFT JOIN catalog.regions ON catalog.regions.id = tenant.clusters.region_id
WHERE
    tenant.clusters.id = @cluster_id;
