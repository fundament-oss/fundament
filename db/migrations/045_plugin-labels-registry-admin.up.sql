SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "plugin_labels_select_admin" ON "appstore"."plugin_labels"
	AS PERMISSIVE
	FOR SELECT
	TO fun_marketplace_admin_api
	USING ((deleted IS NULL));

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "plugin_labels_select_registry" ON "appstore"."plugin_labels"
	AS PERMISSIVE
	FOR SELECT
	TO fun_marketplace_registry_api
	USING (((deleted IS NULL) AND (EXISTS ( SELECT 1
   FROM appstore.plugins
  WHERE ((plugins.id = plugin_labels.plugin_id) AND (plugins.organization_id = authn.current_organization_id()))))));

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."plugin_labels" TO "fun_marketplace_admin_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."plugin_labels" TO "fun_marketplace_registry_api";

