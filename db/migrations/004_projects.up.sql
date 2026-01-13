SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

ALTER INDEX "tenant"."namespaces_uq_name" RENAME TO "pgschemadiff_tmpidx_namespaces_uq_name_KlPi0e0UQIesqenGttfPzQ";

ALTER INDEX "tenant"."projects_uq_organization_name" RENAME TO "pgschemadiff_tmpidx_projects_uq_organiza_Ufkr$ahvTRas_rORdnO$UQ";

CREATE TABLE "tenant"."namespaces_projects" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"namespace_id" uuid,
	"project_id" uuid,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

CREATE POLICY "nss_projects_namespace_organization_isolation" ON "tenant"."namespaces_projects"
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING ((EXISTS ( SELECT 1
   FROM (tenant.namespaces
     LEFT JOIN tenant.clusters ON ((namespaces.cluster_id = clusters.id)))
  WHERE ((namespaces.id = namespaces_projects.namespace_id) AND (clusters.organization_id = (current_setting('app.current_organization_id'::text))::uuid)))));

CREATE POLICY "nss_projects_project_organization_isolation" ON "tenant"."namespaces_projects"
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING ((EXISTS ( SELECT 1
   FROM tenant.projects
  WHERE ((projects.id = namespaces_projects.project_id) AND (projects.organization_id = (current_setting('app.current_organization_id'::text))::uuid)))));

ALTER TABLE "tenant"."namespaces_projects" ENABLE ROW LEVEL SECURITY;

CREATE UNIQUE INDEX namespaces_projects_pk ON tenant.namespaces_projects USING btree (id);

ALTER TABLE "tenant"."namespaces_projects" ADD CONSTRAINT "namespaces_projects_pk" PRIMARY KEY USING INDEX "namespaces_projects_pk";

CREATE UNIQUE INDEX namespaces_projects_uq ON tenant.namespaces_projects USING btree (project_id, namespace_id, deleted) NULLS NOT DISTINCT;

ALTER TABLE "tenant"."namespaces_projects" ADD CONSTRAINT "namespaces_projects_uq" UNIQUE USING INDEX "namespaces_projects_uq";

CREATE INDEX nss_projects_idx_active ON tenant.namespaces_projects USING btree (((deleted IS NULL))) INCLUDE (namespace_id, project_id);

ALTER TABLE "tenant"."namespaces" DROP CONSTRAINT "namespaces_fk_project";

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

ALTER TABLE "tenant"."namespaces_projects" ADD CONSTRAINT "namespaces_projects_fk_project" FOREIGN KEY (project_id) REFERENCES tenant.projects(id) NOT VALID;

ALTER TABLE "tenant"."namespaces_projects" VALIDATE CONSTRAINT "namespaces_projects_fk_project";

/* Hazards:
 - ACQUIRES_ACCESS_EXCLUSIVE_LOCK: Index drops will lock out all accesses to the table. They should be fast.
 - INDEX_DROPPED: Dropping this index means queries that use this index might perform worse because they will no longer will be able to leverage it.
*/
ALTER TABLE "tenant"."namespaces" DROP CONSTRAINT "pgschemadiff_tmpidx_namespaces_uq_name_KlPi0e0UQIesqenGttfPzQ";

/* Hazards:
 - DELETES_DATA: Deletes all values in the column
*/
ALTER TABLE "tenant"."namespaces" DROP COLUMN "project_id";

/* Hazards:
 - ACQUIRES_SHARE_LOCK: Non-concurrent index creates will lock out writes to the table during the duration of the index build.
*/
CREATE UNIQUE INDEX namespaces_uq_name ON tenant.namespaces USING btree (cluster_id, name, deleted) NULLS NOT DISTINCT;

ALTER TABLE "tenant"."namespaces" ADD CONSTRAINT "namespaces_uq_name" UNIQUE USING INDEX "namespaces_uq_name";

ALTER TABLE "tenant"."namespaces_projects" ADD CONSTRAINT "namespaces_projects_fk_namespace" FOREIGN KEY (namespace_id) REFERENCES tenant.namespaces(id) NOT VALID;

ALTER TABLE "tenant"."namespaces_projects" VALIDATE CONSTRAINT "namespaces_projects_fk_namespace";

/* Hazards:
 - ACQUIRES_ACCESS_EXCLUSIVE_LOCK: Index drops will lock out all accesses to the table. They should be fast.
 - INDEX_DROPPED: Dropping this index means queries that use this index might perform worse because they will no longer will be able to leverage it.
*/
ALTER TABLE "tenant"."projects" DROP CONSTRAINT "pgschemadiff_tmpidx_projects_uq_organiza_Ufkr$ahvTRas_rORdnO$UQ";


-- Statements generated automatically, please review:
ALTER TABLE tenant.namespaces_projects OWNER TO fun_fundament_api;
