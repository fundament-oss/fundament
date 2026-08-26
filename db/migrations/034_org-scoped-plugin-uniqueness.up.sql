SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

ALTER INDEX "appstore"."plugins_uq_name" RENAME TO "pgschemadiff_tmpidx_plugins_uq_name_CVHxrJ7CQoGi4W0mGgWawQ";

ALTER TABLE "appstore"."plugins" ADD CONSTRAINT "plugins_ck_name" CHECK(((name ~ '^[a-z][a-z0-9-]*[a-z0-9]$'::text) AND (name !~ '--'::text))) NOT VALID;

ALTER TABLE "appstore"."plugins" VALIDATE CONSTRAINT "plugins_ck_name";

/* Hazards:
 - ACQUIRES_SHARE_LOCK: Non-concurrent index creates will lock out writes to the table during the duration of the index build.
*/
CREATE UNIQUE INDEX plugins_uq_name ON appstore.plugins USING btree (organization_id, name, deleted) NULLS NOT DISTINCT;

ALTER TABLE "appstore"."plugins" ADD CONSTRAINT "plugins_uq_name" UNIQUE USING INDEX "plugins_uq_name";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "organizations_select_plugin_publishers" ON "tenant"."organizations"
	AS PERMISSIVE
	FOR SELECT
	TO fun_fundament_api
	USING ((EXISTS ( SELECT 1
   FROM appstore.plugins
  WHERE ((plugins.organization_id = organizations.id) AND (plugins.deleted IS NULL)))));

ALTER TABLE "tenant"."organizations" DROP CONSTRAINT "organizations_ck_name";

ALTER TABLE "tenant"."organizations" ADD CONSTRAINT "organizations_ck_name" CHECK(((name ~ '^[a-z][a-z0-9-]*[a-z0-9]$'::text) AND (name !~ '--'::text))) NOT VALID;

ALTER TABLE "tenant"."organizations" VALIDATE CONSTRAINT "organizations_ck_name";

/* Hazards:
 - ACQUIRES_ACCESS_EXCLUSIVE_LOCK: Index drops will lock out all accesses to the table. They should be fast.
 - INDEX_DROPPED: Dropping this index means queries that use this index might perform worse because they will no longer will be able to leverage it.
*/
ALTER TABLE "appstore"."plugins" DROP CONSTRAINT "pgschemadiff_tmpidx_plugins_uq_name_CVHxrJ7CQoGi4W0mGgWawQ";

