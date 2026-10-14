SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

CREATE OR REPLACE FUNCTION tenant.platform_overview()
 RETURNS TABLE(organization_name text, organization_alias text, clusters bigint, projects bigint, namespaces bigint)
 LANGUAGE sql
 STABLE PARALLEL SAFE SECURITY DEFINER
 SET search_path TO 'pg_catalog', 'pg_temp'
AS $function$
-- The public platform page (FUN-23) reads through here only: its role has no
-- table grants, so names and counts are all it can ever see. Counts follow the
-- chain organization -> cluster -> project -> namespace with every level live.
-- The seeded system organization owns first-party plugins and is no tenant.
SELECT
    tenant.organizations.name,
    tenant.organizations.alias,
    count(DISTINCT tenant.clusters.id),
    count(DISTINCT tenant.projects.id),
    count(DISTINCT tenant.namespaces.id)
FROM tenant.organizations
LEFT JOIN tenant.clusters
    ON tenant.clusters.organization_id = tenant.organizations.id
    AND tenant.clusters.deleted IS NULL
LEFT JOIN tenant.projects
    ON tenant.projects.cluster_id = tenant.clusters.id
    AND tenant.projects.deleted IS NULL
LEFT JOIN tenant.namespaces
    ON tenant.namespaces.project_id = tenant.projects.id
    AND tenant.namespaces.deleted IS NULL
WHERE tenant.organizations.deleted IS NULL
    AND tenant.organizations.id <> '019b4000-0000-7000-8000-000000000000'::uuid
GROUP BY tenant.organizations.id, tenant.organizations.name, tenant.organizations.alias
ORDER BY tenant.organizations.alias, tenant.organizations.name
$function$
;

CREATE TABLE "tenant"."cluster_snapshots" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"cluster_id" uuid NOT NULL,
	"observed" timestamp with time zone,
	"nodes" integer DEFAULT 0 NOT NULL,
	"nodes_ready" integer DEFAULT 0 NOT NULL,
	"pods" integer DEFAULT 0 NOT NULL,
	"pod_allocatable" integer DEFAULT 0 NOT NULL,
	"cpu_allocatable_m" bigint DEFAULT 0 NOT NULL,
	"memory_allocatable_mi" bigint DEFAULT 0 NOT NULL,
	"last_error" text COLLATE "pg_catalog"."default",
	"last_error_at" timestamp with time zone,
	"created" timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE "tenant"."cluster_snapshots" ADD CONSTRAINT "cluster_snapshots_ck_counts" CHECK(((nodes >= 0) AND (nodes_ready >= 0) AND (nodes_ready <= nodes) AND (pods >= 0) AND (pod_allocatable >= 0) AND (cpu_allocatable_m >= 0) AND (memory_allocatable_mi >= 0)));

ALTER TABLE "tenant"."cluster_snapshots" ADD CONSTRAINT "cluster_snapshots_ck_last_error" CHECK(((last_error IS NULL) = (last_error_at IS NULL)));

CREATE POLICY "cluster_snapshots_cluster_worker_policy" ON "tenant"."cluster_snapshots"
	AS PERMISSIVE
	FOR ALL
	TO fun_cluster_worker
	USING (true);

CREATE POLICY "cluster_snapshots_select_platform_admin" ON "tenant"."cluster_snapshots"
	AS PERMISSIVE
	FOR SELECT
	TO fun_platform_admin_api
	USING (true);

ALTER TABLE "tenant"."cluster_snapshots" ENABLE ROW LEVEL SECURITY;

GRANT INSERT ON "tenant"."cluster_snapshots" TO "fun_cluster_worker";

GRANT SELECT ON "tenant"."cluster_snapshots" TO "fun_cluster_worker";

GRANT UPDATE ON "tenant"."cluster_snapshots" TO "fun_cluster_worker";

GRANT SELECT ON "tenant"."cluster_snapshots" TO "fun_platform_admin_api";

CREATE UNIQUE INDEX cluster_snapshots_pk ON tenant.cluster_snapshots USING btree (id);

ALTER TABLE "tenant"."cluster_snapshots" ADD CONSTRAINT "cluster_snapshots_pk" PRIMARY KEY USING INDEX "cluster_snapshots_pk";

CREATE UNIQUE INDEX cluster_snapshots_uq_cluster ON tenant.cluster_snapshots USING btree (cluster_id);

ALTER TABLE "tenant"."cluster_snapshots" ADD CONSTRAINT "cluster_snapshots_uq_cluster" UNIQUE USING INDEX "cluster_snapshots_uq_cluster";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "clusters_select_platform_admin" ON "tenant"."clusters"
	AS PERMISSIVE
	FOR SELECT
	TO fun_platform_admin_api
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."clusters" TO "fun_platform_admin_api";

ALTER TABLE "tenant"."cluster_snapshots" ADD CONSTRAINT "cluster_snapshots_fk_cluster" FOREIGN KEY (cluster_id) REFERENCES tenant.clusters(id) NOT VALID;

ALTER TABLE "tenant"."cluster_snapshots" VALIDATE CONSTRAINT "cluster_snapshots_fk_cluster";

CREATE TABLE "tenant"."gardener_shoots" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"shoot_uid" text COLLATE "pg_catalog"."default" NOT NULL,
	"shoot_name" text COLLATE "pg_catalog"."default" NOT NULL,
	"shoot_namespace" text COLLATE "pg_catalog"."default" NOT NULL,
	"cluster_id" uuid NOT NULL,
	"kubernetes_version" text COLLATE "pg_catalog"."default" NOT NULL,
	"region" text COLLATE "pg_catalog"."default" NOT NULL,
	"shoot_created" timestamp with time zone NOT NULL,
	"first_seen" timestamp with time zone DEFAULT now() NOT NULL,
	"last_seen" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

CREATE POLICY "gardener_shoots_cluster_worker_policy" ON "tenant"."gardener_shoots"
	AS PERMISSIVE
	FOR ALL
	TO fun_cluster_worker
	USING (true);

CREATE POLICY "gardener_shoots_select_platform_admin" ON "tenant"."gardener_shoots"
	AS PERMISSIVE
	FOR SELECT
	TO fun_platform_admin_api
	USING (true);

ALTER TABLE "tenant"."gardener_shoots" ENABLE ROW LEVEL SECURITY;

GRANT INSERT ON "tenant"."gardener_shoots" TO "fun_cluster_worker";

GRANT SELECT ON "tenant"."gardener_shoots" TO "fun_cluster_worker";

GRANT UPDATE ON "tenant"."gardener_shoots" TO "fun_cluster_worker";

GRANT SELECT ON "tenant"."gardener_shoots" TO "fun_platform_admin_api";

CREATE UNIQUE INDEX gardener_shoots_pk ON tenant.gardener_shoots USING btree (id);

ALTER TABLE "tenant"."gardener_shoots" ADD CONSTRAINT "gardener_shoots_pk" PRIMARY KEY USING INDEX "gardener_shoots_pk";

CREATE UNIQUE INDEX gardener_shoots_uq_shoot_uid ON tenant.gardener_shoots USING btree (shoot_uid, deleted) NULLS NOT DISTINCT;

ALTER TABLE "tenant"."gardener_shoots" ADD CONSTRAINT "gardener_shoots_uq_shoot_uid" UNIQUE USING INDEX "gardener_shoots_uq_shoot_uid";

CREATE INDEX gardener_shoots_idx_cluster_id ON tenant.gardener_shoots USING btree (cluster_id) WHERE (deleted IS NULL);

CREATE TABLE "tenant"."namespace_snapshots" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"namespace_id" uuid NOT NULL,
	"observed" timestamp with time zone NOT NULL,
	"pods" integer NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE "tenant"."namespace_snapshots" ADD CONSTRAINT "namespace_snapshots_ck_pods" CHECK((pods >= 0));

CREATE POLICY "namespace_snapshots_cluster_worker_policy" ON "tenant"."namespace_snapshots"
	AS PERMISSIVE
	FOR ALL
	TO fun_cluster_worker
	USING (true);

CREATE POLICY "namespace_snapshots_select_platform_admin" ON "tenant"."namespace_snapshots"
	AS PERMISSIVE
	FOR SELECT
	TO fun_platform_admin_api
	USING (true);

ALTER TABLE "tenant"."namespace_snapshots" ENABLE ROW LEVEL SECURITY;

GRANT INSERT ON "tenant"."namespace_snapshots" TO "fun_cluster_worker";

GRANT SELECT ON "tenant"."namespace_snapshots" TO "fun_cluster_worker";

GRANT UPDATE ON "tenant"."namespace_snapshots" TO "fun_cluster_worker";

GRANT SELECT ON "tenant"."namespace_snapshots" TO "fun_platform_admin_api";

CREATE UNIQUE INDEX namespace_snapshots_pk ON tenant.namespace_snapshots USING btree (id);

ALTER TABLE "tenant"."namespace_snapshots" ADD CONSTRAINT "namespace_snapshots_pk" PRIMARY KEY USING INDEX "namespace_snapshots_pk";

CREATE UNIQUE INDEX namespace_snapshots_uq_namespace ON tenant.namespace_snapshots USING btree (namespace_id);

ALTER TABLE "tenant"."namespace_snapshots" ADD CONSTRAINT "namespace_snapshots_uq_namespace" UNIQUE USING INDEX "namespace_snapshots_uq_namespace";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "namespaces_select_platform_admin" ON "tenant"."namespaces"
	AS PERMISSIVE
	FOR SELECT
	TO fun_platform_admin_api
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."namespaces" TO "fun_platform_admin_api";

ALTER TABLE "tenant"."namespace_snapshots" ADD CONSTRAINT "namespace_snapshots_fk_namespace" FOREIGN KEY (namespace_id) REFERENCES tenant.namespaces(id) NOT VALID;

ALTER TABLE "tenant"."namespace_snapshots" VALIDATE CONSTRAINT "namespace_snapshots_fk_namespace";

CREATE TABLE "tenant"."node_pool_snapshots" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"node_pool_id" uuid NOT NULL,
	"observed" timestamp with time zone NOT NULL,
	"nodes" integer NOT NULL,
	"nodes_ready" integer NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE "tenant"."node_pool_snapshots" ADD CONSTRAINT "node_pool_snapshots_ck_counts" CHECK(((nodes >= 0) AND (nodes_ready >= 0) AND (nodes_ready <= nodes)));

CREATE POLICY "node_pool_snapshots_cluster_worker_policy" ON "tenant"."node_pool_snapshots"
	AS PERMISSIVE
	FOR ALL
	TO fun_cluster_worker
	USING (true);

CREATE POLICY "node_pool_snapshots_select_platform_admin" ON "tenant"."node_pool_snapshots"
	AS PERMISSIVE
	FOR SELECT
	TO fun_platform_admin_api
	USING (true);

ALTER TABLE "tenant"."node_pool_snapshots" ENABLE ROW LEVEL SECURITY;

GRANT INSERT ON "tenant"."node_pool_snapshots" TO "fun_cluster_worker";

GRANT SELECT ON "tenant"."node_pool_snapshots" TO "fun_cluster_worker";

GRANT UPDATE ON "tenant"."node_pool_snapshots" TO "fun_cluster_worker";

GRANT SELECT ON "tenant"."node_pool_snapshots" TO "fun_platform_admin_api";

CREATE UNIQUE INDEX node_pool_snapshots_pk ON tenant.node_pool_snapshots USING btree (id);

ALTER TABLE "tenant"."node_pool_snapshots" ADD CONSTRAINT "node_pool_snapshots_pk" PRIMARY KEY USING INDEX "node_pool_snapshots_pk";

CREATE UNIQUE INDEX node_pool_snapshots_uq_node_pool ON tenant.node_pool_snapshots USING btree (node_pool_id);

ALTER TABLE "tenant"."node_pool_snapshots" ADD CONSTRAINT "node_pool_snapshots_uq_node_pool" UNIQUE USING INDEX "node_pool_snapshots_uq_node_pool";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "node_pools_select_platform_admin" ON "tenant"."node_pools"
	AS PERMISSIVE
	FOR SELECT
	TO fun_platform_admin_api
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."node_pools" TO "fun_platform_admin_api";

ALTER TABLE "tenant"."node_pool_snapshots" ADD CONSTRAINT "node_pool_snapshots_fk_node_pool" FOREIGN KEY (node_pool_id) REFERENCES tenant.node_pools(id) NOT VALID;

ALTER TABLE "tenant"."node_pool_snapshots" VALIDATE CONSTRAINT "node_pool_snapshots_fk_node_pool";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "organizations_select_platform_admin" ON "tenant"."organizations"
	AS PERMISSIVE
	FOR SELECT
	TO fun_platform_admin_api
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."organizations" TO "fun_platform_admin_api";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "projects_select_platform_admin" ON "tenant"."projects"
	AS PERMISSIVE
	FOR SELECT
	TO fun_platform_admin_api
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."projects" TO "fun_platform_admin_api";


-- Statements generated automatically, please review:
ALTER FUNCTION tenant.platform_overview() OWNER TO fun_owner;
ALTER TABLE tenant.cluster_snapshots OWNER TO fun_owner;
ALTER TABLE tenant.gardener_shoots OWNER TO fun_owner;
ALTER TABLE tenant.namespace_snapshots OWNER TO fun_owner;
ALTER TABLE tenant.node_pool_snapshots OWNER TO fun_owner;

-- Hand-added statements. trek's diff emits table-wide grants but not
-- schema-level or function-level ones, even when they are modelled in the .dbm,
-- so every statement below must be re-added whenever the migration is
-- regenerated.
--
-- tenant USAGE for both platform roles (FUN-23): the public role reaches no
-- table and needs the schema only to call platform_overview(); the admin role
-- reads the tenant tables its policies above open up. EXECUTE is PUBLIC by
-- default; the explicit grant records that the public role depends on it.
GRANT USAGE ON SCHEMA "tenant" TO "fun_platform_public_api";
GRANT USAGE ON SCHEMA "tenant" TO "fun_platform_admin_api";
GRANT EXECUTE ON FUNCTION tenant.platform_overview() TO "fun_platform_public_api";
