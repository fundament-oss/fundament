SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "tenant"."cluster_outbox" TO "fun_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "clusters_authn_api_policy" ON "tenant"."clusters"
	AS PERMISSIVE
	FOR SELECT
	TO fun_authn_api
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."clusters" TO "fun_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "project_members_authn_api_policy" ON "tenant"."project_members"
	AS PERMISSIVE
	FOR SELECT
	TO fun_authn_api
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."project_members" TO "fun_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "projects_authn_api_policy" ON "tenant"."projects"
	AS PERMISSIVE
	FOR SELECT
	TO fun_authn_api
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."projects" TO "fun_authn_api";

