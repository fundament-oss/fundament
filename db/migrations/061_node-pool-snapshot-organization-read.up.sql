SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "node_pool_snapshots_select_organization" ON "tenant"."node_pool_snapshots"
	AS PERMISSIVE
	FOR SELECT
	TO fun_fundament_api
	USING ((EXISTS ( SELECT 1
   FROM tenant.node_pools
  WHERE ((node_pools.id = node_pool_snapshots.node_pool_id) AND authn.is_cluster_in_organization(node_pools.cluster_id)))));

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."node_pool_snapshots" TO "fun_fundament_api";

