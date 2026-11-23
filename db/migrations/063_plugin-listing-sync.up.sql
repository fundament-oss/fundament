SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "categories_plugins_delete_admin" ON "appstore"."categories_plugins"
	AS PERMISSIVE
	FOR DELETE
	TO fun_marketplace_admin_api
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "categories_plugins_insert_admin" ON "appstore"."categories_plugins"
	AS PERMISSIVE
	FOR INSERT
	TO fun_marketplace_admin_api
	WITH CHECK (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT DELETE ON "appstore"."categories_plugins" TO "fun_marketplace_admin_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "appstore"."categories_plugins" TO "fun_marketplace_admin_api";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "plugin_documentation_links_all_admin" ON "appstore"."plugin_documentation_links"
	AS PERMISSIVE
	FOR ALL
	TO fun_marketplace_admin_api
	USING (true)
	WITH CHECK (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "appstore"."plugin_documentation_links" TO "fun_marketplace_admin_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."plugin_documentation_links" TO "fun_marketplace_admin_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "appstore"."plugin_documentation_links" TO "fun_marketplace_admin_api";

/* Hazards:
 - AUTHZ_UPDATE: Removing a permissive policy could cause queries to fail if not correctly configured.
*/
DROP POLICY "plugin_documentation_links_all_registry" ON "appstore"."plugin_documentation_links";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "plugin_documentation_links_select_registry" ON "appstore"."plugin_documentation_links"
	AS PERMISSIVE
	FOR SELECT
	TO fun_marketplace_registry_api
	USING ((EXISTS ( SELECT 1
   FROM appstore.plugins
  WHERE ((plugins.id = plugin_documentation_links.plugin_id) AND (plugins.organization_id = authn.current_organization_id())))));

/* Hazards:
 - AUTHZ_UPDATE: Revoking privileges could cause queries to fail if not correctly configured.
*/
REVOKE INSERT ON "appstore"."plugin_documentation_links" FROM "fun_marketplace_registry_api";

/* Hazards:
 - AUTHZ_UPDATE: Revoking privileges could cause queries to fail if not correctly configured.
*/
REVOKE UPDATE ON "appstore"."plugin_documentation_links" FROM "fun_marketplace_registry_api";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "plugin_features_all_admin" ON "appstore"."plugin_features"
	AS PERMISSIVE
	FOR ALL
	TO fun_marketplace_admin_api
	USING (true)
	WITH CHECK (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "appstore"."plugin_features" TO "fun_marketplace_admin_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."plugin_features" TO "fun_marketplace_admin_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "appstore"."plugin_features" TO "fun_marketplace_admin_api";

/* Hazards:
 - AUTHZ_UPDATE: Removing a permissive policy could cause queries to fail if not correctly configured.
*/
DROP POLICY "plugin_features_all_registry" ON "appstore"."plugin_features";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "plugin_features_select_registry" ON "appstore"."plugin_features"
	AS PERMISSIVE
	FOR SELECT
	TO fun_marketplace_registry_api
	USING ((EXISTS ( SELECT 1
   FROM appstore.plugins
  WHERE ((plugins.id = plugin_features.plugin_id) AND (plugins.organization_id = authn.current_organization_id())))));

/* Hazards:
 - AUTHZ_UPDATE: Revoking privileges could cause queries to fail if not correctly configured.
*/
REVOKE INSERT ON "appstore"."plugin_features" FROM "fun_marketplace_registry_api";

/* Hazards:
 - AUTHZ_UPDATE: Revoking privileges could cause queries to fail if not correctly configured.
*/
REVOKE UPDATE ON "appstore"."plugin_features" FROM "fun_marketplace_registry_api";

ALTER TABLE "appstore"."plugins" ADD COLUMN "listing_draft" jsonb;

ALTER TABLE "appstore"."plugins" ADD COLUMN "logo" text COLLATE "pg_catalog"."default" DEFAULT ''::text NOT NULL;

ALTER TABLE "appstore"."plugins" ADD COLUMN "logo_dark" text COLLATE "pg_catalog"."default" DEFAULT ''::text NOT NULL;

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "plugins_update_admin" ON "appstore"."plugins"
	AS PERMISSIVE
	FOR UPDATE
	TO fun_marketplace_admin_api
	USING (true)
	WITH CHECK (true);

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "plugins_tags_delete_admin" ON "appstore"."plugins_tags"
	AS PERMISSIVE
	FOR DELETE
	TO fun_marketplace_admin_api
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "plugins_tags_insert_admin" ON "appstore"."plugins_tags"
	AS PERMISSIVE
	FOR INSERT
	TO fun_marketplace_admin_api
	WITH CHECK (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT DELETE ON "appstore"."plugins_tags" TO "fun_marketplace_admin_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "appstore"."plugins_tags" TO "fun_marketplace_admin_api";

ALTER TABLE "appstore"."submissions" ADD COLUMN "listing_draft" jsonb;

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "appstore"."tags" TO "fun_marketplace_admin_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "authz"."outbox" TO "fun_marketplace_admin_api";


/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ("display_name") ON "appstore"."plugins" TO "fun_marketplace_admin_api";
GRANT UPDATE ("description_short") ON "appstore"."plugins" TO "fun_marketplace_admin_api";
GRANT UPDATE ("repository_url") ON "appstore"."plugins" TO "fun_marketplace_admin_api";
GRANT UPDATE ("license") ON "appstore"."plugins" TO "fun_marketplace_admin_api";
GRANT UPDATE ("updated") ON "appstore"."plugins" TO "fun_marketplace_admin_api";

GRANT USAGE ON SCHEMA "authz" TO "fun_marketplace_admin_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ("description") ON "appstore"."plugins" TO "fun_marketplace_admin_api";
GRANT UPDATE ("image") ON "appstore"."plugins" TO "fun_marketplace_admin_api";
GRANT UPDATE ("author_name") ON "appstore"."plugins" TO "fun_marketplace_admin_api";
GRANT UPDATE ("author_url") ON "appstore"."plugins" TO "fun_marketplace_admin_api";
GRANT UPDATE ("logo") ON "appstore"."plugins" TO "fun_marketplace_admin_api";
GRANT UPDATE ("logo_dark") ON "appstore"."plugins" TO "fun_marketplace_admin_api";
GRANT UPDATE ("listing_draft") ON "appstore"."plugins" TO "fun_marketplace_admin_api";

GRANT INSERT ("listing_draft") ON "appstore"."submissions" TO "fun_marketplace_registry_api";
