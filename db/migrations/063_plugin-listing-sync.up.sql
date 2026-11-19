SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

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

