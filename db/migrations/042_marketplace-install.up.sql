SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

-- Hand-added statements. trek's diff emits table-wide grants but not
-- schema-level ones, even when they are modelled in the .dbm (see 035 and 041),
-- so these must be re-added whenever this migration is regenerated.
--
-- Schema USAGE for the install role (FUN-22): without it every table grant
-- below is unusable. tenant: GetPluginDefinition resolves the publishing
-- organization by name.
GRANT USAGE ON SCHEMA "appstore" TO "fun_marketplace_install_api";
GRANT USAGE ON SCHEMA "tenant" TO "fun_marketplace_install_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."categories" TO "fun_marketplace_install_api";

/* Hazards:
 - AUTHZ_UPDATE: Altering a policy could cause queries to fail if not correctly configured or allow unauthorized access to data.
*/
ALTER POLICY "categories_plugins_select_catalog" ON "appstore"."categories_plugins"
	TO fun_marketplace_catalog_api, fun_marketplace_install_api;

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."categories_plugins" TO "fun_marketplace_install_api";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "plugin_allowed_organizations_select_install" ON "appstore"."plugin_allowed_organizations"
	AS PERMISSIVE
	FOR SELECT
	TO fun_marketplace_install_api
	USING ((organization_id = authn.current_organization_id()));

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."plugin_allowed_organizations" TO "fun_marketplace_install_api";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "plugin_definitions_select_install" ON "appstore"."plugin_definitions"
	AS PERMISSIVE
	FOR SELECT
	TO fun_marketplace_install_api
	USING (((deleted IS NULL) AND (EXISTS ( SELECT 1
   FROM appstore.plugins
  WHERE ((plugins.id = plugin_definitions.plugin_id) AND (plugins.deleted IS NULL) AND ((plugin_definitions.published IS NOT NULL) OR (plugins.organization_id = authn.current_organization_id())))))));

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."plugin_definitions" TO "fun_marketplace_install_api";

/* Hazards:
 - AUTHZ_UPDATE: Altering a policy could cause queries to fail if not correctly configured or allow unauthorized access to data.
*/
ALTER POLICY "plugin_documentation_links_select_catalog" ON "appstore"."plugin_documentation_links"
	TO fun_marketplace_catalog_api, fun_marketplace_install_api;

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."plugin_documentation_links" TO "fun_marketplace_install_api";

/* Hazards:
 - AUTHZ_UPDATE: Altering a policy could cause queries to fail if not correctly configured or allow unauthorized access to data.
*/
ALTER POLICY "plugin_features_select_catalog" ON "appstore"."plugin_features"
	TO fun_marketplace_catalog_api, fun_marketplace_install_api;

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."plugin_features" TO "fun_marketplace_install_api";

/* Hazards:
 - AUTHZ_UPDATE: Altering a policy could cause queries to fail if not correctly configured or allow unauthorized access to data.
*/
ALTER POLICY "plugin_labels_select_catalog" ON "appstore"."plugin_labels"
	TO fun_marketplace_catalog_api, fun_marketplace_install_api;

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."plugin_labels" TO "fun_marketplace_install_api";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "plugins_select_install" ON "appstore"."plugins"
	AS PERMISSIVE
	FOR SELECT
	TO fun_marketplace_install_api
	USING (((deleted IS NULL) AND ((visibility = 'public'::text) OR (organization_id = authn.current_organization_id()) OR (EXISTS ( SELECT 1
   FROM appstore.plugin_allowed_organizations
  WHERE ((plugin_allowed_organizations.plugin_id = plugins.id) AND (plugin_allowed_organizations.organization_id = authn.current_organization_id())))))));

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."plugins" TO "fun_marketplace_install_api";

/* Hazards:
 - AUTHZ_UPDATE: Altering a policy could cause queries to fail if not correctly configured or allow unauthorized access to data.
*/
ALTER POLICY "plugins_tags_select_catalog" ON "appstore"."plugins_tags"
	TO fun_marketplace_catalog_api, fun_marketplace_install_api;

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."plugins_tags" TO "fun_marketplace_install_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."tags" TO "fun_marketplace_install_api";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "organizations_select_install" ON "tenant"."organizations"
	AS PERMISSIVE
	FOR SELECT
	TO fun_marketplace_install_api
	USING (((deleted IS NULL) AND (EXISTS ( SELECT 1
   FROM appstore.plugins
  WHERE ((plugins.organization_id = organizations.id) AND (plugins.deleted IS NULL))))));

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."organizations" TO "fun_marketplace_install_api";

