SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

ALTER INDEX "tenant"."namespaces_uq_name" RENAME TO "pgschemadiff_tmpidx_namespaces_uq_name_E_B48TrmQl$2g62SuYcG8g";

ALTER TABLE "tenant"."namespaces" ADD COLUMN "cluster_id" uuid;
UPDATE tenant.namespaces SET cluster_id = (SELECT cluster_id FROM tenant.projects WHERE id = project_id);
ALTER TABLE "tenant"."namespaces" ALTER COLUMN "cluster_id" SET NOT NULL;

ALTER TABLE "tenant"."namespaces" DROP CONSTRAINT "namespaces_ck_name";

ALTER TABLE "tenant"."namespaces" ADD CONSTRAINT "namespaces_fk_cluster" FOREIGN KEY (cluster_id) REFERENCES tenant.clusters(id) NOT VALID;

ALTER TABLE "tenant"."namespaces" VALIDATE CONSTRAINT "namespaces_fk_cluster";

/* Hazards:
 - ACQUIRES_SHARE_LOCK: Non-concurrent index creates will lock out writes to the table during the duration of the index build.
*/
CREATE UNIQUE INDEX namespaces_uq_name ON tenant.namespaces USING btree (cluster_id, name, deleted) NULLS NOT DISTINCT;

ALTER TABLE "tenant"."namespaces" ADD CONSTRAINT "namespaces_uq_name" UNIQUE USING INDEX "namespaces_uq_name";

CREATE TABLE "tenant"."project_limits" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"project_id" uuid NOT NULL,
	"default_memory_request_mi" integer,
	"default_memory_limit_mi" integer,
	"default_cpu_request_m" integer,
	"default_cpu_limit_m" integer,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

ALTER TABLE "tenant"."project_limits" ADD CONSTRAINT "project_limits_ck_cpu_limit_gte_request" CHECK(((default_cpu_limit_m IS NULL) OR (default_cpu_request_m IS NULL) OR (default_cpu_limit_m >= default_cpu_request_m)));

ALTER TABLE "tenant"."project_limits" ADD CONSTRAINT "project_limits_ck_default_cpu_limit_m" CHECK(((default_cpu_limit_m IS NULL) OR (default_cpu_limit_m > 0)));

ALTER TABLE "tenant"."project_limits" ADD CONSTRAINT "project_limits_ck_default_cpu_request_m" CHECK(((default_cpu_request_m IS NULL) OR (default_cpu_request_m > 0)));

ALTER TABLE "tenant"."project_limits" ADD CONSTRAINT "project_limits_ck_default_memory_limit_mi" CHECK(((default_memory_limit_mi IS NULL) OR (default_memory_limit_mi > 0)));

ALTER TABLE "tenant"."project_limits" ADD CONSTRAINT "project_limits_ck_default_memory_request_mi" CHECK(((default_memory_request_mi IS NULL) OR (default_memory_request_mi > 0)));

ALTER TABLE "tenant"."project_limits" ADD CONSTRAINT "project_limits_ck_memory_limit_gte_request" CHECK(((default_memory_limit_mi IS NULL) OR (default_memory_request_mi IS NULL) OR (default_memory_limit_mi >= default_memory_request_mi)));

CREATE POLICY "project_limits_project_policy" ON "tenant"."project_limits"
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING (authn.is_project_in_organization(project_id));

ALTER TABLE "tenant"."project_limits" ENABLE ROW LEVEL SECURITY;

GRANT INSERT ON "tenant"."project_limits" TO "fun_fundament_api";

GRANT SELECT ON "tenant"."project_limits" TO "fun_fundament_api";

GRANT UPDATE ON "tenant"."project_limits" TO "fun_fundament_api";

CREATE UNIQUE INDEX project_limits_pk ON tenant.project_limits USING btree (id);

ALTER TABLE "tenant"."project_limits" ADD CONSTRAINT "project_limits_pk" PRIMARY KEY USING INDEX "project_limits_pk";

CREATE UNIQUE INDEX project_limits_uq_project ON tenant.project_limits USING btree (project_id, deleted) NULLS NOT DISTINCT;

ALTER TABLE "tenant"."project_limits" ADD CONSTRAINT "project_limits_uq_project" UNIQUE USING INDEX "project_limits_uq_project";

ALTER TABLE "tenant"."project_limits" ADD CONSTRAINT "project_limits_fk_project" FOREIGN KEY (project_id) REFERENCES tenant.projects(id) NOT VALID;

ALTER TABLE "tenant"."project_limits" VALIDATE CONSTRAINT "project_limits_fk_project";

/* Hazards:
 - ACQUIRES_ACCESS_EXCLUSIVE_LOCK: Index drops will lock out all accesses to the table. They should be fast.
 - INDEX_DROPPED: Dropping this index means queries that use this index might perform worse because they will no longer will be able to leverage it.
*/
ALTER TABLE "tenant"."namespaces" DROP CONSTRAINT "pgschemadiff_tmpidx_namespaces_uq_name_E_B48TrmQl$2g62SuYcG8g";


-- Statements generated automatically, please review:
ALTER TABLE tenant.project_limits OWNER TO fun_owner;
