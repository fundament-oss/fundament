SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

-- Hand-added statements. trek's diff emits table-wide grants but not
-- schema-level or column-level ones, even when they are modelled in the .dbm,
-- so every statement marked "hand-added" in this file must be re-added whenever
-- the migration is regenerated.
--
-- Schema USAGE for the admin role:
--   appstore: without it every table grant below is unusable.
--   tenant: ListPublishers resolves publishing organizations, which
--     organization-api cannot serve to a reviewer (FUN-20).
GRANT USAGE ON SCHEMA "appstore" TO "fun_marketplace_admin_api";
GRANT USAGE ON SCHEMA "tenant" TO "fun_marketplace_admin_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."categories" TO "fun_marketplace_admin_api";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "categories_plugins_select_admin" ON "appstore"."categories_plugins"
	AS PERMISSIVE
	FOR SELECT
	TO fun_marketplace_admin_api
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."categories_plugins" TO "fun_marketplace_admin_api";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "plugin_definitions_select_admin" ON "appstore"."plugin_definitions"
	AS PERMISSIVE
	FOR SELECT
	TO fun_marketplace_admin_api
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
-- Split from a single FOR ALL policy: with per-command policies, INSERT and
-- DELETE stay structurally impossible for this role even if a table-wide
-- grant ever slips in through a regenerated migration.
CREATE POLICY "plugin_definitions_update_admin" ON "appstore"."plugin_definitions"
	AS PERMISSIVE
	FOR UPDATE
	TO fun_marketplace_admin_api
	USING (true)
	WITH CHECK (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."plugin_definitions" TO "fun_marketplace_admin_api";

-- Hand-added, column-scoped. A decision moves a version's status out of
-- PENDING and stamps published on approval. hash, manifest and image are the
-- consent record FUN-20 makes immutable, so the grant, not just stack 3's Go,
-- refuses them.
GRANT UPDATE ("status") ON "appstore"."plugin_definitions" TO "fun_marketplace_admin_api";
GRANT UPDATE ("published") ON "appstore"."plugin_definitions" TO "fun_marketplace_admin_api";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "plugins_select_admin" ON "appstore"."plugins"
	AS PERMISSIVE
	FOR SELECT
	TO fun_marketplace_admin_api
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."plugins" TO "fun_marketplace_admin_api";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "plugins_tags_select_admin" ON "appstore"."plugins_tags"
	AS PERMISSIVE
	FOR SELECT
	TO fun_marketplace_admin_api
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."plugins_tags" TO "fun_marketplace_admin_api";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "submissions_select_admin" ON "appstore"."submissions"
	AS PERMISSIVE
	FOR SELECT
	TO fun_marketplace_admin_api
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
-- Split from a single FOR ALL policy, as on plugin_definitions above.
CREATE POLICY "submissions_update_admin" ON "appstore"."submissions"
	AS PERMISSIVE
	FOR UPDATE
	TO fun_marketplace_admin_api
	USING (true)
	WITH CHECK (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."submissions" TO "fun_marketplace_admin_api";

-- Hand-added, column-scoped. A decision records who decided, when and why, and
-- closes the round. Which version a round reviews and who submitted it are the
-- publisher's alone, so the reviewer cannot rewrite them.
GRANT UPDATE ("reviewer_user_id") ON "appstore"."submissions" TO "fun_marketplace_admin_api";
GRANT UPDATE ("reviewed") ON "appstore"."submissions" TO "fun_marketplace_admin_api";
GRANT UPDATE ("closed") ON "appstore"."submissions" TO "fun_marketplace_admin_api";
GRANT UPDATE ("rejection_reason") ON "appstore"."submissions" TO "fun_marketplace_admin_api";
GRANT UPDATE ("feedback") ON "appstore"."submissions" TO "fun_marketplace_admin_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."tags" TO "fun_marketplace_admin_api";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "organizations_select_admin" ON "tenant"."organizations"
	AS PERMISSIVE
	FOR SELECT
	TO fun_marketplace_admin_api
	USING (((deleted IS NULL) AND (EXISTS ( SELECT 1
   FROM appstore.plugins
  WHERE ((plugins.organization_id = organizations.id) AND (plugins.deleted IS NULL))))));

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."organizations" TO "fun_marketplace_admin_api";

