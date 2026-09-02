SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "preset_plugins_all_api" ON "appstore"."preset_plugins"
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "preset_plugins_select_catalog" ON "appstore"."preset_plugins"
	AS PERMISSIVE
	FOR SELECT
	TO fun_marketplace_catalog_api
	USING ((EXISTS ( SELECT 1
   FROM appstore.plugins
  WHERE (plugins.id = preset_plugins.plugin_id))));

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."preset_plugins" TO "fun_marketplace_catalog_api";

/* Hazards:
 - AUTHZ_UPDATE: Enabling RLS on a table could cause queries to fail if not correctly configured.
*/
ALTER TABLE "appstore"."preset_plugins" ENABLE ROW LEVEL SECURITY;

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "presets_all_api" ON "appstore"."presets"
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "presets_select_catalog" ON "appstore"."presets"
	AS PERMISSIVE
	FOR SELECT
	TO fun_marketplace_catalog_api
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."presets" TO "fun_marketplace_catalog_api";

/* Hazards:
 - AUTHZ_UPDATE: Enabling RLS on a table could cause queries to fail if not correctly configured.
*/
ALTER TABLE "appstore"."presets" ENABLE ROW LEVEL SECURITY;

