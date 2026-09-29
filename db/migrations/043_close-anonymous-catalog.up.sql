SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - AUTHZ_UPDATE: Altering a policy could cause queries to fail if not correctly configured or allow unauthorized access to data.
*/
ALTER POLICY "plugin_definitions_select_catalog" ON "appstore"."plugin_definitions"
	USING (((deleted IS NULL) AND (published IS NOT NULL)));

