SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "namespaces_organization_policy" ON "tenant"."namespaces"
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING ((EXISTS ( SELECT 1
   FROM tenant.clusters
  WHERE ((clusters.id = namespaces.cluster_id) AND (clusters.organization_id = (current_setting('app.current_organization_id'::text))::uuid)))));

/* Hazards:
 - AUTHZ_UPDATE: Enabling RLS on a table could cause queries to fail if not correctly configured.
*/
ALTER TABLE "tenant"."namespaces" ENABLE ROW LEVEL SECURITY;

