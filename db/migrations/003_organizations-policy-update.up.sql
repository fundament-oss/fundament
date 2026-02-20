SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "organizations_update_policy" ON "tenant"."organizations"
	AS PERMISSIVE
	FOR UPDATE
	TO fun_fundament_api
	USING (authn.is_organization_member(id));
