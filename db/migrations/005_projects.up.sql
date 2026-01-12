SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

ALTER INDEX "tenant"."namespaces_uq_name" RENAME TO "pgschemadiff_tmpidx_namespaces_uq_name_ZDKFe3YyQqWDEp1vmuiGKg";

ALTER INDEX "tenant"."projects_uq_organization_name" RENAME TO "pgschemadiff_tmpidx_projects_uq_organiza_HdSFniPcRWyfQet3xewY6A";

/* Hazards:
 - ACQUIRES_SHARE_LOCK: Non-concurrent index creates will lock out writes to the table during the duration of the index build.
*/
CREATE UNIQUE INDEX namespaces_uq_name ON tenant.namespaces USING btree (cluster_id, name, deleted) NULLS NOT DISTINCT;

ALTER TABLE "tenant"."namespaces" ADD CONSTRAINT "namespaces_uq_name" UNIQUE USING INDEX "namespaces_uq_name";

ALTER TABLE "tenant"."projects" ADD COLUMN "deleted" timestamp with time zone;

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "projects_organization_isolation" ON "tenant"."projects"
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING ((organization_id = (current_setting('app.current_organization_id'::text))::uuid));

/* Hazards:
 - AUTHZ_UPDATE: Enabling RLS on a table could cause queries to fail if not correctly configured.
*/
ALTER TABLE "tenant"."projects" ENABLE ROW LEVEL SECURITY;

/* Hazards:
 - ACQUIRES_SHARE_LOCK: Non-concurrent index creates will lock out writes to the table during the duration of the index build.
*/
CREATE UNIQUE INDEX projects_uq_organization_name ON tenant.projects USING btree (organization_id, name, deleted);

ALTER TABLE "tenant"."projects" ADD CONSTRAINT "projects_uq_organization_name" UNIQUE USING INDEX "projects_uq_organization_name";

/* Hazards:
 - ACQUIRES_ACCESS_EXCLUSIVE_LOCK: Index drops will lock out all accesses to the table. They should be fast.
 - INDEX_DROPPED: Dropping this index means queries that use this index might perform worse because they will no longer will be able to leverage it.
*/
ALTER TABLE "tenant"."namespaces" DROP CONSTRAINT "pgschemadiff_tmpidx_namespaces_uq_name_ZDKFe3YyQqWDEp1vmuiGKg";

/* Hazards:
 - ACQUIRES_ACCESS_EXCLUSIVE_LOCK: Index drops will lock out all accesses to the table. They should be fast.
 - INDEX_DROPPED: Dropping this index means queries that use this index might perform worse because they will no longer will be able to leverage it.
*/
ALTER TABLE "tenant"."projects" DROP CONSTRAINT "pgschemadiff_tmpidx_projects_uq_organiza_HdSFniPcRWyfQet3xewY6A";

