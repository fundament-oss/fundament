-- name: ClusterGetForPluginMachinery :one
-- Get the owning organization and shoot status of a cluster: the organization
-- is stamped as per-shoot identity onto the plugin-controller Deployment, the
-- status gates provisioning on shoot readiness.
SELECT tenant.clusters.organization_id, tenant.clusters.shoot_status
FROM tenant.clusters
WHERE tenant.clusters.id = @id
  AND tenant.clusters.deleted IS NULL;
