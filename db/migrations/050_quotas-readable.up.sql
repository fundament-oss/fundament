SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "organization_machine_quotas_organization_select" ON "tenant"."organization_machine_quotas"
	AS PERMISSIVE
	FOR SELECT
	TO fun_fundament_api
	USING ((organization_id = authn.current_organization_id()));

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."organization_machine_quotas" TO "fun_fundament_api";

