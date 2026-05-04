SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

CREATE TABLE "tenant"."organization_limits" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"organization_id" uuid NOT NULL,
	"max_nodes_per_cluster" integer,
	"max_node_pools_per_cluster" integer,
	"max_nodes_per_node_pool" integer,
	"default_memory_request_mi" integer,
	"default_memory_limit_mi" integer,
	"default_cpu_request_m" integer,
	"default_cpu_limit_m" integer,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

ALTER TABLE "tenant"."organization_limits" ADD CONSTRAINT "organization_limits_ck_cpu_limit_gte_request" CHECK(((default_cpu_limit_m IS NULL) OR (default_cpu_request_m IS NULL) OR (default_cpu_limit_m >= default_cpu_request_m)));

ALTER TABLE "tenant"."organization_limits" ADD CONSTRAINT "organization_limits_ck_default_cpu_limit_m" CHECK(((default_cpu_limit_m IS NULL) OR (default_cpu_limit_m > 0)));

ALTER TABLE "tenant"."organization_limits" ADD CONSTRAINT "organization_limits_ck_default_cpu_request_m" CHECK(((default_cpu_request_m IS NULL) OR (default_cpu_request_m > 0)));

ALTER TABLE "tenant"."organization_limits" ADD CONSTRAINT "organization_limits_ck_default_memory_limit_mi" CHECK(((default_memory_limit_mi IS NULL) OR (default_memory_limit_mi > 0)));

ALTER TABLE "tenant"."organization_limits" ADD CONSTRAINT "organization_limits_ck_default_memory_request_mi" CHECK(((default_memory_request_mi IS NULL) OR (default_memory_request_mi > 0)));

ALTER TABLE "tenant"."organization_limits" ADD CONSTRAINT "organization_limits_ck_max_node_pools_per_cluster" CHECK(((max_node_pools_per_cluster IS NULL) OR (max_node_pools_per_cluster > 0)));

ALTER TABLE "tenant"."organization_limits" ADD CONSTRAINT "organization_limits_ck_max_nodes_per_cluster" CHECK(((max_nodes_per_cluster IS NULL) OR (max_nodes_per_cluster > 0)));

ALTER TABLE "tenant"."organization_limits" ADD CONSTRAINT "organization_limits_ck_max_nodes_per_node_pool" CHECK(((max_nodes_per_node_pool IS NULL) OR (max_nodes_per_node_pool > 0)));

ALTER TABLE "tenant"."organization_limits" ADD CONSTRAINT "organization_limits_ck_memory_limit_gte_request" CHECK(((default_memory_limit_mi IS NULL) OR (default_memory_request_mi IS NULL) OR (default_memory_limit_mi >= default_memory_request_mi)));

CREATE POLICY "organization_limits_organization_policy" ON "tenant"."organization_limits"
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING ((organization_id = authn.current_organization_id()));

ALTER TABLE "tenant"."organization_limits" ENABLE ROW LEVEL SECURITY;

CREATE UNIQUE INDEX organization_limits_pk ON tenant.organization_limits USING btree (id);

ALTER TABLE "tenant"."organization_limits" ADD CONSTRAINT "organization_limits_pk" PRIMARY KEY USING INDEX "organization_limits_pk";

CREATE UNIQUE INDEX organization_limits_uq_org ON tenant.organization_limits USING btree (organization_id, deleted) NULLS NOT DISTINCT;

ALTER TABLE "tenant"."organization_limits" ADD CONSTRAINT "organization_limits_uq_org" UNIQUE USING INDEX "organization_limits_uq_org";

ALTER TABLE "tenant"."organization_limits" ADD CONSTRAINT "organization_limits_fk_organization" FOREIGN KEY (organization_id) REFERENCES tenant.organizations(id) NOT VALID;

ALTER TABLE "tenant"."organization_limits" VALIDATE CONSTRAINT "organization_limits_fk_organization";


/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "tenant"."organization_limits" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."organization_limits" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "tenant"."organization_limits" TO "fun_fundament_api";

-- Statements generated automatically, please review:
ALTER TABLE tenant.organization_limits OWNER TO fun_owner;
