-- name: OrganizationLimitsGetByOrgID :one
-- Load the active node caps for an organization. Used by the cluster sync to
-- clamp/validate the generated Shoot worker pools. Returns pgx.ErrNoRows when
-- the organization has no active limits row (all caps unlimited); a NULL cap
-- means that individual cap is unlimited.
SELECT
    max_nodes_per_cluster,
    max_node_pools_per_cluster,
    max_nodes_per_node_pool
FROM tenant.organization_limits
WHERE organization_id = @organization_id
  AND deleted IS NULL;
