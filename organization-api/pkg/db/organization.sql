
-- name: OrganizationGetByID :one
-- Membership is asserted here rather than left to RLS: the plugin-publisher
-- policy also grants SELECT on this table, and this endpoint must not expose it.
SELECT id, name, alias, created
FROM tenant.organizations
WHERE id = $1
  AND (tenant.organizations.id = authn.current_organization_id()
       OR authn.is_organization_member(tenant.organizations.id));

-- name: OrganizationUpdate :one
UPDATE tenant.organizations
SET alias = $2
WHERE id = $1
RETURNING id, name, alias, created;

-- name: OrganizationList :many
-- Membership is asserted here rather than left to RLS: the plugin-publisher
-- policy also grants SELECT on this table, and listing must stay member-scoped.
SELECT id, name, alias, created
FROM tenant.organizations
WHERE tenant.organizations.id = authn.current_organization_id()
   OR authn.is_organization_member(tenant.organizations.id)
ORDER BY created;

-- name: OrganizationQuotaClustersGet :one
-- The cluster quota next to what counts against it; the function is the one
-- the clusters_tr_verify_quota trigger counts with.
SELECT
  quota_clusters,
  tenant.organization_clusters_in_use(id)::bigint AS clusters_in_use
FROM tenant.organizations
WHERE id = @id
  AND deleted IS NULL;

-- name: OrganizationMachineQuotaList :many
-- Every offering the organization has a quota row for, and every offering it
-- has nodes of without one (pools from before quotas existed), as quota 0.
-- Same shape as funops' MachineQuotaList; the policy on the quota rows scopes
-- it to the current organization.
SELECT
    catalog.regions.name AS region,
    catalog.machine_types.name AS machine_type,
    coalesce(tenant.organization_machine_quotas.max_nodes, 0)::integer AS max_nodes,
    tenant.organization_nodes_in_use(@organization_id, catalog.region_machine_types.id)::bigint AS nodes_in_use
FROM catalog.region_machine_types
JOIN catalog.regions ON catalog.regions.id = catalog.region_machine_types.region_id
JOIN catalog.machine_types ON catalog.machine_types.id = catalog.region_machine_types.machine_type_id
LEFT JOIN tenant.organization_machine_quotas
    ON tenant.organization_machine_quotas.region_machine_type_id = catalog.region_machine_types.id
   AND tenant.organization_machine_quotas.organization_id = @organization_id
WHERE tenant.organization_machine_quotas.organization_id IS NOT NULL
   OR tenant.organization_nodes_in_use(@organization_id, catalog.region_machine_types.id) > 0
ORDER BY region, machine_type;

-- name: OrganizationIsLive :one
-- Tells a failed ClusterCreate apart: the organization was soft-deleted by
-- funops after the request was authorised.
SELECT EXISTS (
    SELECT 1
    FROM tenant.organizations
    WHERE id = $1
      AND deleted IS NULL
);
