SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - AUTHZ_UPDATE: Altering a policy could cause queries to fail if not correctly configured or allow unauthorized access to data.
*/
ALTER POLICY "plugin_definitions_select_catalog" ON "appstore"."plugin_definitions"
	USING ((deleted IS NULL));

/* Hazards:
 - AUTHZ_UPDATE: Altering a policy could cause queries to fail if not correctly configured or allow unauthorized access to data.
*/
ALTER POLICY "plugins_select_catalog" ON "appstore"."plugins"
	USING (((deleted IS NULL) AND (visibility = 'public'::text)));

/* Hazards:
 - AUTHZ_UPDATE: Altering a policy could cause queries to fail if not correctly configured or allow unauthorized access to data.
*/
ALTER POLICY "organizations_select_catalog" ON "tenant"."organizations"
	USING (((deleted IS NULL) AND (EXISTS ( SELECT 1
   FROM appstore.plugins
  WHERE ((plugins.organization_id = organizations.id) AND (plugins.deleted IS NULL) AND (plugins.visibility = 'public'::text))))));

