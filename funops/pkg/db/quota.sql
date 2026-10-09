-- Quotas: what an organization may build. The triggers on tenant.clusters and
-- tenant.node_pools enforce them (clusters_tr_verify_quota,
-- node_pools_tr_verify_quota); these queries set and show them. What is in use
-- comes from the same functions the triggers count with.

-- name: QuotaClustersGet :one
SELECT
  quota_clusters,
  tenant.organization_clusters_in_use(id)::bigint AS clusters_in_use
FROM tenant.organizations
WHERE id = @id
  AND deleted IS NULL;

-- name: QuotaClustersSet :execrows
UPDATE tenant.organizations
SET quota_clusters = @quota_clusters
WHERE id = @id
  AND deleted IS NULL;

-- name: RegionMachineTypeResolve :one
-- Resolve (region name, machine type name) to the region_machine_types row; no
-- row means the machine type is not offered in that region.
SELECT
    catalog.region_machine_types.id
FROM catalog.region_machine_types
JOIN catalog.regions ON catalog.regions.id = catalog.region_machine_types.region_id
JOIN catalog.machine_types ON catalog.machine_types.id = catalog.region_machine_types.machine_type_id
WHERE catalog.regions.name = @region_name
  AND catalog.machine_types.name = @machine_type_name;

-- name: MachineQuotaSet :execrows
-- One row per organization and offering, never removed: setting 0 is how a
-- quota is taken away, and reads the same as no row to the trigger.
-- The organization row is locked the way node_pools_tr_verify_quota locks it,
-- so a pool growing against the old quota and this lowering cannot both
-- commit: one waits for the other. No row means the organization is gone.
WITH locked AS (
    SELECT tenant.organizations.id
    FROM tenant.organizations
    WHERE tenant.organizations.id = @organization_id
      AND tenant.organizations.deleted IS NULL
    FOR NO KEY UPDATE
)
INSERT INTO tenant.organization_machine_quotas (organization_id, region_machine_type_id, max_nodes)
SELECT locked.id, @region_machine_type_id, @max_nodes
FROM locked
ON CONFLICT (organization_id, region_machine_type_id) DO UPDATE
SET max_nodes = EXCLUDED.max_nodes,
    updated = now();

-- name: MachineQuotaList :many
SELECT
    catalog.regions.name AS region,
    catalog.machine_types.name AS machine_type,
    tenant.organization_machine_quotas.max_nodes,
    tenant.organization_nodes_in_use(
        tenant.organization_machine_quotas.organization_id,
        tenant.organization_machine_quotas.region_machine_type_id
    )::bigint AS nodes_in_use,
    tenant.organization_machine_quotas.updated
FROM tenant.organization_machine_quotas
JOIN catalog.region_machine_types ON catalog.region_machine_types.id = tenant.organization_machine_quotas.region_machine_type_id
JOIN catalog.regions ON catalog.regions.id = catalog.region_machine_types.region_id
JOIN catalog.machine_types ON catalog.machine_types.id = catalog.region_machine_types.machine_type_id
WHERE tenant.organization_machine_quotas.organization_id = @organization_id
ORDER BY region, machine_type;
