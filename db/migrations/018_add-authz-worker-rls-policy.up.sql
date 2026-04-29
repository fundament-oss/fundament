SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "organizations_users_authz_worker_policy" ON "tenant"."organizations_users"
	AS PERMISSIVE
	FOR SELECT
	TO fun_authz_worker
	USING (true);

