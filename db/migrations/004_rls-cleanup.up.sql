SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - AUTHZ_UPDATE: Altering a policy could cause queries to fail if not correctly configured or allow unauthorized access to data.
*/
ALTER POLICY "api_keys_organization_policy" ON "authn"."api_keys"
	USING ((organization_id = (current_setting('app.current_organization_id'::text))::uuid));

/* Hazards:
 - AUTHZ_UPDATE: Removing a permissive policy could cause queries to fail if not correctly configured.
*/
DROP POLICY "organizations_select_policy" ON "tenant"."organizations";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "organizations_organization_policy" ON "tenant"."organizations"
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING ((id = authn.current_organization_id()));

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "organizations_user_select_policy" ON "tenant"."organizations"
	AS PERMISSIVE
	FOR SELECT
	TO fun_fundament_api
	USING (authn.is_organization_member(id));

/* Hazards:
 - AUTHZ_UPDATE: Removing a permissive policy could cause queries to fail if not correctly configured.
*/
DROP POLICY "organizations_users_insert_policy" ON "tenant"."organizations_users";

/* Hazards:
 - AUTHZ_UPDATE: Removing a permissive policy could cause queries to fail if not correctly configured.
*/
DROP POLICY "organizations_users_select_policy" ON "tenant"."organizations_users";

/* Hazards:
 - AUTHZ_UPDATE: Removing a permissive policy could cause queries to fail if not correctly configured.
*/
DROP POLICY "organizations_users_update_policy" ON "tenant"."organizations_users";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "organizations_users_organization_policy" ON "tenant"."organizations_users"
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING ((organization_id = authn.current_organization_id()));

/* Hazards:
 - AUTHZ_UPDATE: Removing a permissive policy could cause queries to fail if not correctly configured.
*/
DROP POLICY "project_members_insert_policy" ON "tenant"."project_members";

/* Hazards:
 - AUTHZ_UPDATE: Removing a permissive policy could cause queries to fail if not correctly configured.
*/
DROP POLICY "project_members_select_policy" ON "tenant"."project_members";

/* Hazards:
 - AUTHZ_UPDATE: Removing a permissive policy could cause queries to fail if not correctly configured.
*/
DROP POLICY "project_members_update_policy" ON "tenant"."project_members";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "project_members_organization_policy" ON "tenant"."project_members"
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING (authn.is_project_in_organization(project_id));

/* Hazards:
 - AUTHZ_UPDATE: Removing a permissive policy could cause queries to fail if not correctly configured.
*/
DROP POLICY "projects_delete_policy" ON "tenant"."projects";

/* Hazards:
 - AUTHZ_UPDATE: Removing a permissive policy could cause queries to fail if not correctly configured.
*/
DROP POLICY "projects_insert_policy" ON "tenant"."projects";

/* Hazards:
 - AUTHZ_UPDATE: Removing a permissive policy could cause queries to fail if not correctly configured.
*/
DROP POLICY "projects_select_policy" ON "tenant"."projects";

/* Hazards:
 - AUTHZ_UPDATE: Removing a permissive policy could cause queries to fail if not correctly configured.
*/
DROP POLICY "projects_update_policy" ON "tenant"."projects";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "projects_organization_policy" ON "tenant"."projects"
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING (authn.is_cluster_in_organization(cluster_id));

DROP FUNCTION "authn"."is_project_member"(p_project_id uuid, p_user_id uuid, p_role text);

DROP FUNCTION "authn"."is_user_in_organization"(p_user_id uuid);

DROP FUNCTION "tenant"."project_has_members"(p_project_id uuid);

