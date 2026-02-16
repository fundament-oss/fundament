SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

CREATE SCHEMA "tenant";


CREATE TABLE "tenant"."clusters" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"organization_id" uuid NOT NULL,
	"name" text COLLATE "pg_catalog"."default" NOT NULL,
	"region" text COLLATE "pg_catalog"."default" NOT NULL,
	"kubernetes_version" text COLLATE "pg_catalog"."default" NOT NULL,
	"status" text COLLATE "pg_catalog"."default" NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

ALTER TABLE "tenant"."clusters" ADD CONSTRAINT "clusters_ck_status" CHECK((status = ANY (ARRAY['unspecified'::text, 'provisioning'::text, 'starting'::text, 'running'::text, 'upgrading'::text, 'error'::text, 'stopping'::text, 'stopped'::text])));

CREATE POLICY "organization_isolation" ON "tenant"."clusters"
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING ((organization_id = (current_setting('app.current_organization_id'::text))::uuid));

ALTER TABLE "tenant"."clusters" ENABLE ROW LEVEL SECURITY;

CREATE UNIQUE INDEX clusters_pk ON tenant.clusters USING btree (id);

ALTER TABLE "tenant"."clusters" ADD CONSTRAINT "clusters_pk" PRIMARY KEY USING INDEX "clusters_pk";

CREATE UNIQUE INDEX clusters_uq_name ON tenant.clusters USING btree (organization_id, name, deleted) NULLS NOT DISTINCT;

ALTER TABLE "tenant"."clusters" ADD CONSTRAINT "clusters_uq_name" UNIQUE USING INDEX "clusters_uq_name";

CREATE OR REPLACE FUNCTION tenant.clusters_tr_verify_deleted()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
	IF EXISTS (
		SELECT 1
		FROM tenant.namespaces
		WHERE
			cluster_id = NEW.id
			AND deleted IS NULL
	) THEN
		RAISE EXCEPTION 'Cannot delete cluster with undeleted namespaces';
	END IF;
	RETURN NEW;
END;
$function$
;

CREATE CONSTRAINT TRIGGER verify_deleted
AFTER INSERT OR UPDATE ON tenant.clusters
NOT DEFERRABLE INITIALLY IMMEDIATE
FOR EACH ROW
EXECUTE FUNCTION tenant.clusters_tr_verify_deleted();

CREATE TABLE "tenant"."namespaces" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"project_id" uuid NOT NULL,
	"cluster_id" uuid NOT NULL,
	"name" text COLLATE "pg_catalog"."default" NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

ALTER TABLE "tenant"."namespaces" ADD CONSTRAINT "namespaces_ck_name" CHECK((name = name));

ALTER TABLE "tenant"."namespaces" ADD CONSTRAINT "namespaces_fk_cluster" FOREIGN KEY (cluster_id) REFERENCES tenant.clusters(id) NOT VALID;

ALTER TABLE "tenant"."namespaces" VALIDATE CONSTRAINT "namespaces_fk_cluster";

CREATE UNIQUE INDEX namespaces_pk ON tenant.namespaces USING btree (id);

ALTER TABLE "tenant"."namespaces" ADD CONSTRAINT "namespaces_pk" PRIMARY KEY USING INDEX "namespaces_pk";

CREATE UNIQUE INDEX namespaces_uq_name ON tenant.namespaces USING btree (project_id, name, deleted) NULLS NOT DISTINCT;

ALTER TABLE "tenant"."namespaces" ADD CONSTRAINT "namespaces_uq_name" UNIQUE USING INDEX "namespaces_uq_name";

CREATE TABLE "tenant"."organizations" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"name" text COLLATE "pg_catalog"."default" NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL
);

CREATE UNIQUE INDEX organizations_pk ON tenant.organizations USING btree (id);

ALTER TABLE "tenant"."organizations" ADD CONSTRAINT "organizations_pk" PRIMARY KEY USING INDEX "organizations_pk";

CREATE UNIQUE INDEX organizations_uq_name ON tenant.organizations USING btree (name);

ALTER TABLE "tenant"."organizations" ADD CONSTRAINT "organizations_uq_name" UNIQUE USING INDEX "organizations_uq_name";

ALTER TABLE "tenant"."clusters" ADD CONSTRAINT "clusters_fk_organization" FOREIGN KEY (organization_id) REFERENCES tenant.organizations(id) NOT VALID;

ALTER TABLE "tenant"."clusters" VALIDATE CONSTRAINT "clusters_fk_organization";

CREATE TABLE "tenant"."projects" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"organization_id" uuid NOT NULL,
	"name" text COLLATE "pg_catalog"."default" NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE "tenant"."projects" ADD CONSTRAINT "projects_fk_organization" FOREIGN KEY (organization_id) REFERENCES tenant.organizations(id) NOT VALID;

ALTER TABLE "tenant"."projects" VALIDATE CONSTRAINT "projects_fk_organization";

CREATE UNIQUE INDEX projects_pk ON tenant.projects USING btree (id);

ALTER TABLE "tenant"."projects" ADD CONSTRAINT "projects_pk" PRIMARY KEY USING INDEX "projects_pk";

CREATE UNIQUE INDEX projects_uq_organization_name ON tenant.projects USING btree (organization_id, name);

ALTER TABLE "tenant"."projects" ADD CONSTRAINT "projects_uq_organization_name" UNIQUE USING INDEX "projects_uq_organization_name";

ALTER TABLE "tenant"."namespaces" ADD CONSTRAINT "namespaces_fk_project" FOREIGN KEY (project_id) REFERENCES tenant.projects(id) NOT VALID;

ALTER TABLE "tenant"."namespaces" VALIDATE CONSTRAINT "namespaces_fk_project";

CREATE TABLE "tenant"."users" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"organization_id" uuid NOT NULL,
	"name" text COLLATE "pg_catalog"."default" NOT NULL,
	"external_id" text COLLATE "pg_catalog"."default" NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE "tenant"."users" ADD CONSTRAINT "users_fk_organization" FOREIGN KEY (organization_id) REFERENCES tenant.organizations(id) NOT VALID;

ALTER TABLE "tenant"."users" VALIDATE CONSTRAINT "users_fk_organization";

CREATE UNIQUE INDEX users_pk ON tenant.users USING btree (id);

ALTER TABLE "tenant"."users" ADD CONSTRAINT "users_pk" PRIMARY KEY USING INDEX "users_pk";

CREATE UNIQUE INDEX users_uq_external_id ON tenant.users USING btree (external_id);

ALTER TABLE "tenant"."users" ADD CONSTRAINT "users_uq_external_id" UNIQUE USING INDEX "users_uq_external_id";

SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

CREATE TABLE "tenant"."node_pools" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"cluster_id" uuid NOT NULL,
	"name" text COLLATE "pg_catalog"."default" NOT NULL,
	"machine_type" text COLLATE "pg_catalog"."default" NOT NULL,
	"autoscale_min" integer NOT NULL,
	"autoscale_max" integer NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

CREATE POLICY "node_pools_organization_policy" ON "tenant"."node_pools"
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING ((EXISTS ( SELECT 1
   FROM tenant.clusters
  WHERE ((clusters.id = node_pools.cluster_id) AND (clusters.organization_id = (current_setting('app.current_organization_id'::text))::uuid)))));

ALTER TABLE "tenant"."node_pools" ENABLE ROW LEVEL SECURITY;

ALTER TABLE "tenant"."node_pools" ADD CONSTRAINT "node_pools_fk_cluster" FOREIGN KEY (cluster_id) REFERENCES tenant.clusters(id) NOT VALID;

ALTER TABLE "tenant"."node_pools" VALIDATE CONSTRAINT "node_pools_fk_cluster";

CREATE UNIQUE INDEX node_pools_pk ON tenant.node_pools USING btree (id);

ALTER TABLE "tenant"."node_pools" ADD CONSTRAINT "node_pools_pk" PRIMARY KEY USING INDEX "node_pools_pk";

CREATE UNIQUE INDEX node_pools_uq_name ON tenant.node_pools USING btree (cluster_id, name, deleted) NULLS NOT DISTINCT;

ALTER TABLE "tenant"."node_pools" ADD CONSTRAINT "node_pools_uq_name" UNIQUE USING INDEX "node_pools_uq_name";


SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

CREATE TABLE "tenant"."installs" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"cluster_id" uuid NOT NULL,
	"plugin_id" uuid NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

CREATE POLICY "install_organization_policy" ON "tenant"."installs"
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING ((EXISTS ( SELECT 1
   FROM tenant.clusters
  WHERE ((clusters.id = installs.cluster_id) AND (clusters.organization_id = (current_setting('app.current_organization_id'::text))::uuid)))));

ALTER TABLE "tenant"."installs" ENABLE ROW LEVEL SECURITY;

ALTER TABLE "tenant"."installs" ADD CONSTRAINT "installs_fk_cluster" FOREIGN KEY (cluster_id) REFERENCES tenant.clusters(id) NOT VALID;

ALTER TABLE "tenant"."installs" VALIDATE CONSTRAINT "installs_fk_cluster";

CREATE UNIQUE INDEX installs_pk ON tenant.installs USING btree (id);

ALTER TABLE "tenant"."installs" ADD CONSTRAINT "installs_pk" PRIMARY KEY USING INDEX "installs_pk";

CREATE UNIQUE INDEX installs_uq ON tenant.installs USING btree (cluster_id, plugin_id, deleted) NULLS NOT DISTINCT;

ALTER TABLE "tenant"."installs" ADD CONSTRAINT "installs_uq" UNIQUE USING INDEX "installs_uq";

CREATE TABLE "tenant"."plugins" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"name" text COLLATE "pg_catalog"."default" NOT NULL
);

CREATE UNIQUE INDEX plugins_pk ON tenant.plugins USING btree (id);

ALTER TABLE "tenant"."plugins" ADD CONSTRAINT "plugins_pk" PRIMARY KEY USING INDEX "plugins_pk";

CREATE UNIQUE INDEX plugins_uq_name ON tenant.plugins USING btree (name);

ALTER TABLE "tenant"."plugins" ADD CONSTRAINT "plugins_uq_name" UNIQUE USING INDEX "plugins_uq_name";

ALTER TABLE "tenant"."installs" ADD CONSTRAINT "installs_fk_plugin" FOREIGN KEY (plugin_id) REFERENCES tenant.plugins(id) NOT VALID;

ALTER TABLE "tenant"."installs" VALIDATE CONSTRAINT "installs_fk_plugin";


-- Statements generated automatically, please review:
ALTER TABLE tenant.installs OWNER TO fun_fundament_api;
ALTER TABLE tenant.plugins OWNER TO fun_fundament_api;

SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

CREATE SCHEMA "appstore";

CREATE TABLE "appstore"."categories" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"name" text COLLATE "pg_catalog"."default" NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

CREATE UNIQUE INDEX categories_pk ON appstore.categories USING btree (id);

ALTER TABLE "appstore"."categories" ADD CONSTRAINT "categories_pk" PRIMARY KEY USING INDEX "categories_pk";

CREATE UNIQUE INDEX categories_uq_name ON appstore.categories USING btree (name, deleted) NULLS NOT DISTINCT;

ALTER TABLE "appstore"."categories" ADD CONSTRAINT "categories_uq_name" UNIQUE USING INDEX "categories_uq_name";

CREATE TABLE "appstore"."categories_plugins" (
	"plugin_id" uuid NOT NULL,
	"category_id" uuid NOT NULL
);

ALTER TABLE "appstore"."categories_plugins" ADD CONSTRAINT "plugins_categories_category_id" FOREIGN KEY (category_id) REFERENCES appstore.categories(id) NOT VALID;

ALTER TABLE "appstore"."categories_plugins" VALIDATE CONSTRAINT "plugins_categories_category_id";

CREATE TABLE "appstore"."installs" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"cluster_id" uuid NOT NULL,
	"plugin_id" uuid NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

CREATE POLICY "install_organization_policy" ON "appstore"."installs"
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING ((EXISTS ( SELECT 1
   FROM tenant.clusters
  WHERE ((clusters.id = installs.cluster_id) AND (clusters.organization_id = (current_setting('app.current_organization_id'::text))::uuid)))));

ALTER TABLE "appstore"."installs" ENABLE ROW LEVEL SECURITY;

CREATE UNIQUE INDEX installs_pk ON appstore.installs USING btree (id);

ALTER TABLE "appstore"."installs" ADD CONSTRAINT "installs_pk" PRIMARY KEY USING INDEX "installs_pk";

CREATE UNIQUE INDEX installs_uq ON appstore.installs USING btree (cluster_id, plugin_id, deleted) NULLS NOT DISTINCT;

ALTER TABLE "appstore"."installs" ADD CONSTRAINT "installs_uq" UNIQUE USING INDEX "installs_uq";

CREATE TABLE "appstore"."plugins" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"name" text COLLATE "pg_catalog"."default" NOT NULL,
	"description" text COLLATE "pg_catalog"."default" NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

CREATE UNIQUE INDEX plugins_pk ON appstore.plugins USING btree (id);

ALTER TABLE "appstore"."plugins" ADD CONSTRAINT "plugins_pk" PRIMARY KEY USING INDEX "plugins_pk";

CREATE UNIQUE INDEX plugins_uq_name ON appstore.plugins USING btree (name, deleted) NULLS NOT DISTINCT;

ALTER TABLE "appstore"."plugins" ADD CONSTRAINT "plugins_uq_name" UNIQUE USING INDEX "plugins_uq_name";

ALTER TABLE "appstore"."categories_plugins" ADD CONSTRAINT "plugins_categories_plugin_id" FOREIGN KEY (plugin_id) REFERENCES appstore.plugins(id) NOT VALID;

ALTER TABLE "appstore"."categories_plugins" VALIDATE CONSTRAINT "plugins_categories_plugin_id";

ALTER TABLE "appstore"."installs" ADD CONSTRAINT "installs_fk_plugin" FOREIGN KEY (plugin_id) REFERENCES appstore.plugins(id) NOT VALID;

ALTER TABLE "appstore"."installs" VALIDATE CONSTRAINT "installs_fk_plugin";

CREATE TABLE "appstore"."plugins_tags" (
	"plugin_id" uuid NOT NULL,
	"tag_id" uuid NOT NULL
);

ALTER TABLE "appstore"."plugins_tags" ADD CONSTRAINT "plugins_tags_plugin_id" FOREIGN KEY (plugin_id) REFERENCES appstore.plugins(id) NOT VALID;

ALTER TABLE "appstore"."plugins_tags" VALIDATE CONSTRAINT "plugins_tags_plugin_id";

CREATE TABLE "appstore"."tags" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"name" text COLLATE "pg_catalog"."default" NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

CREATE UNIQUE INDEX tags_pk ON appstore.tags USING btree (id);

ALTER TABLE "appstore"."tags" ADD CONSTRAINT "tags_pk" PRIMARY KEY USING INDEX "tags_pk";

CREATE UNIQUE INDEX tags_uq_name ON appstore.tags USING btree (name, deleted) NULLS NOT DISTINCT;

ALTER TABLE "appstore"."tags" ADD CONSTRAINT "tags_uq_name" UNIQUE USING INDEX "tags_uq_name";

ALTER TABLE "appstore"."plugins_tags" ADD CONSTRAINT "plugins_tags_tag_id" FOREIGN KEY (tag_id) REFERENCES appstore.tags(id) NOT VALID;

ALTER TABLE "appstore"."plugins_tags" VALIDATE CONSTRAINT "plugins_tags_tag_id";

ALTER TABLE "tenant"."installs" DROP CONSTRAINT "installs_fk_cluster";

ALTER TABLE "appstore"."installs" ADD CONSTRAINT "installs_fk_cluster" FOREIGN KEY (cluster_id) REFERENCES tenant.clusters(id) NOT VALID;

ALTER TABLE "appstore"."installs" VALIDATE CONSTRAINT "installs_fk_cluster";

ALTER TABLE "tenant"."installs" DROP CONSTRAINT "installs_fk_plugin";

SET SESSION statement_timeout = 1200000;

/* Hazards:
 - DELETES_DATA: Deletes all rows in the table (and the table itself)
*/
DROP TABLE "tenant"."installs";

/* Hazards:
 - DELETES_DATA: Deletes all rows in the table (and the table itself)
*/
DROP TABLE "tenant"."plugins";


-- Statements generated automatically, please review:
ALTER SCHEMA appstore OWNER TO fun_fundament_api;
ALTER TABLE appstore.categories OWNER TO fun_fundament_api;
ALTER TABLE appstore.categories_plugins OWNER TO fun_fundament_api;
ALTER TABLE appstore.installs OWNER TO fun_fundament_api;
ALTER TABLE appstore.plugins OWNER TO fun_fundament_api;
ALTER TABLE appstore.plugins_tags OWNER TO fun_fundament_api;
ALTER TABLE appstore.tags OWNER TO fun_fundament_api;

SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

ALTER INDEX "tenant"."projects_uq_organization_name" RENAME TO "pgschemadiff_tmpidx_projects_uq_organiza_Ix$cGeM_QGSHMMM4V9Vpjw";

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
CREATE UNIQUE INDEX projects_uq_organization_name ON tenant.projects USING btree (organization_id, name, deleted) NULLS NOT DISTINCT;

ALTER TABLE "tenant"."projects" ADD CONSTRAINT "projects_uq_organization_name" UNIQUE USING INDEX "projects_uq_organization_name";

/* Hazards:
 - ACQUIRES_ACCESS_EXCLUSIVE_LOCK: Index drops will lock out all accesses to the table. They should be fast.
 - INDEX_DROPPED: Dropping this index means queries that use this index might perform worse because they will no longer will be able to leverage it.
*/
ALTER TABLE "tenant"."projects" DROP CONSTRAINT "pgschemadiff_tmpidx_projects_uq_organiza_Ix$cGeM_QGSHMMM4V9Vpjw";


SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "namespaces_organization_policy" ON "tenant"."namespaces"
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING ((EXISTS ( SELECT 1
   FROM tenant.clusters
  WHERE ((clusters.id = namespaces.cluster_id) AND (clusters.organization_id = (current_setting('app.current_organization_id'::text))::uuid)))));

/* Hazards:
 - AUTHZ_UPDATE: Enabling RLS on a table could cause queries to fail if not correctly configured.
*/
ALTER TABLE "tenant"."namespaces" ENABLE ROW LEVEL SECURITY;


SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - ACQUIRES_SHARE_LOCK: Non-concurrent index creates will lock out writes to the table during the duration of the index build.
*/
CREATE UNIQUE INDEX categories_plugins_pk ON appstore.categories_plugins USING btree (plugin_id, category_id);

ALTER TABLE "appstore"."categories_plugins" ADD CONSTRAINT "categories_plugins_pk" PRIMARY KEY USING INDEX "categories_plugins_pk";

/* Hazards:
 - ACQUIRES_SHARE_LOCK: Non-concurrent index creates will lock out writes to the table during the duration of the index build.
*/
CREATE UNIQUE INDEX plugins_tags_pk ON appstore.plugins_tags USING btree (plugin_id, tag_id);

ALTER TABLE "appstore"."plugins_tags" ADD CONSTRAINT "plugins_tags_pk" PRIMARY KEY USING INDEX "plugins_tags_pk";

CREATE TABLE "appstore"."preset_plugins" (
	"preset_id" uuid NOT NULL,
	"plugin_id" uuid NOT NULL
);

ALTER TABLE "appstore"."preset_plugins" ADD CONSTRAINT "plugins_presets_plugin_id" FOREIGN KEY (plugin_id) REFERENCES appstore.plugins(id) NOT VALID;

ALTER TABLE "appstore"."preset_plugins" VALIDATE CONSTRAINT "plugins_presets_plugin_id";

CREATE UNIQUE INDEX preset_plugins_pk ON appstore.preset_plugins USING btree (preset_id, plugin_id);

ALTER TABLE "appstore"."preset_plugins" ADD CONSTRAINT "preset_plugins_pk" PRIMARY KEY USING INDEX "preset_plugins_pk";

CREATE TABLE "appstore"."presets" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"name" text COLLATE "pg_catalog"."default" NOT NULL,
	"description" text COLLATE "pg_catalog"."default"
);

CREATE UNIQUE INDEX presets_pk ON appstore.presets USING btree (id);

ALTER TABLE "appstore"."presets" ADD CONSTRAINT "presets_pk" PRIMARY KEY USING INDEX "presets_pk";

CREATE UNIQUE INDEX presets_uq_name ON appstore.presets USING btree (name);

ALTER TABLE "appstore"."presets" ADD CONSTRAINT "presets_uq_name" UNIQUE USING INDEX "presets_uq_name";

ALTER TABLE "appstore"."preset_plugins" ADD CONSTRAINT "plugins_presets_preset_id" FOREIGN KEY (preset_id) REFERENCES appstore.presets(id) NOT VALID;

ALTER TABLE "appstore"."preset_plugins" VALIDATE CONSTRAINT "plugins_presets_preset_id";


-- Statements generated automatically, please review:
ALTER TABLE appstore.preset_plugins OWNER TO fun_fundament_api;
ALTER TABLE appstore.presets OWNER TO fun_fundament_api;

SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

CREATE TABLE "appstore"."plugin_documentation_links" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"plugin_id" uuid NOT NULL,
	"title" text COLLATE "pg_catalog"."default" NOT NULL,
	"url_name" text COLLATE "pg_catalog"."default" NOT NULL,
	"url" text COLLATE "pg_catalog"."default" NOT NULL
);

CREATE UNIQUE INDEX plugin_documentation_links_pk ON appstore.plugin_documentation_links USING btree (id);

ALTER TABLE "appstore"."plugin_documentation_links" ADD CONSTRAINT "plugin_documentation_links_pk" PRIMARY KEY USING INDEX "plugin_documentation_links_pk";

ALTER TABLE "appstore"."plugins" ADD COLUMN "author_name" text COLLATE "pg_catalog"."default";

ALTER TABLE "appstore"."plugins" ADD COLUMN "author_url" text COLLATE "pg_catalog"."default";

ALTER TABLE "appstore"."plugins" ADD COLUMN "repository_url" text COLLATE "pg_catalog"."default";

ALTER TABLE "appstore"."plugin_documentation_links" ADD CONSTRAINT "plugin_documentation_links_fk_plugin" FOREIGN KEY (plugin_id) REFERENCES appstore.plugins(id) NOT VALID;

ALTER TABLE "appstore"."plugin_documentation_links" VALIDATE CONSTRAINT "plugin_documentation_links_fk_plugin";


-- Statements generated automatically, please review:
ALTER TABLE appstore.plugin_documentation_links OWNER TO fun_fundament_api;

SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

ALTER TABLE "appstore"."plugins" ADD COLUMN "description_short" text COLLATE "pg_catalog"."default" DEFAULT ''::text NOT NULL;


-- Statements generated automatically, please review:
ALTER SCHEMA tenant OWNER TO fun_owner;
ALTER SCHEMA appstore OWNER TO fun_owner;
ALTER TABLE tenant.clusters OWNER TO fun_owner;
ALTER TABLE tenant.namespaces OWNER TO fun_owner;
ALTER TABLE tenant.node_pools OWNER TO fun_owner;
ALTER TABLE tenant.organizations OWNER TO fun_owner;
ALTER TABLE tenant.projects OWNER TO fun_owner;
ALTER TABLE tenant.users OWNER TO fun_owner;
ALTER TABLE appstore.categories OWNER TO fun_owner;
ALTER TABLE appstore.categories_plugins OWNER TO fun_owner;
ALTER TABLE appstore.installs OWNER TO fun_owner;
ALTER TABLE appstore.plugin_documentation_links OWNER TO fun_owner;
ALTER TABLE appstore.plugins OWNER TO fun_owner;
ALTER TABLE appstore.plugins_tags OWNER TO fun_owner;
ALTER TABLE appstore.preset_plugins OWNER TO fun_owner;
ALTER TABLE appstore.presets OWNER TO fun_owner;
ALTER TABLE appstore.tags OWNER TO fun_owner;

GRANT USAGE
   ON SCHEMA tenant
   TO fun_fundament_api;

GRANT USAGE
   ON SCHEMA appstore
   TO fun_fundament_api;

GRANT USAGE
   ON SCHEMA tenant
   TO fun_authn_api;


/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "tenant"."clusters" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."clusters" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "tenant"."clusters" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "tenant"."namespaces" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."namespaces" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "tenant"."namespaces" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "tenant"."node_pools" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."node_pools" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "tenant"."node_pools" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "tenant"."organizations" TO "fun_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."organizations" TO "fun_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "tenant"."organizations" TO "fun_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "tenant"."organizations" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."organizations" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "tenant"."organizations" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "tenant"."projects" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."projects" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "tenant"."projects" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "tenant"."users" TO "fun_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."users" TO "fun_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "tenant"."users" TO "fun_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "tenant"."users" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."users" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "tenant"."users" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "appstore"."categories" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."categories" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "appstore"."categories" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "appstore"."categories_plugins" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."categories_plugins" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "appstore"."categories_plugins" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "appstore"."installs" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."installs" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "appstore"."installs" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "appstore"."plugin_documentation_links" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."plugin_documentation_links" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "appstore"."plugin_documentation_links" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "appstore"."plugins" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."plugins" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "appstore"."plugins" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "appstore"."plugins_tags" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."plugins_tags" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "appstore"."plugins_tags" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "appstore"."preset_plugins" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."preset_plugins" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "appstore"."preset_plugins" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "appstore"."presets" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."presets" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "appstore"."presets" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "appstore"."tags" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."tags" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "appstore"."tags" TO "fun_fundament_api";

SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

ALTER INDEX "tenant"."users_uq_external_id" RENAME TO "pgschemadiff_tmpidx_users_uq_external_id_bpGqLi6qTPqFhOo7IGQNfw";

ALTER TABLE "tenant"."users" ADD COLUMN "deleted" timestamp with time zone;

ALTER TABLE "tenant"."users" ADD COLUMN "email" text COLLATE "pg_catalog"."default";

ALTER TABLE "tenant"."users" ALTER COLUMN "external_id" DROP NOT NULL;

ALTER TABLE "tenant"."users" ADD COLUMN "role" text COLLATE "pg_catalog"."default" DEFAULT 'viewer'::text NOT NULL;

/* Hazards:
 - ACQUIRES_SHARE_LOCK: Non-concurrent index creates will lock out writes to the table during the duration of the index build.
*/
CREATE UNIQUE INDEX users_uq_external_id ON tenant.users USING btree (external_id, deleted) NULLS NOT DISTINCT;

ALTER TABLE "tenant"."users" ADD CONSTRAINT "users_uq_external_id" UNIQUE USING INDEX "users_uq_external_id";

/* Hazards:
 - ACQUIRES_ACCESS_EXCLUSIVE_LOCK: Index drops will lock out all accesses to the table. They should be fast.
 - INDEX_DROPPED: Dropping this index means queries that use this index might perform worse because they will no longer will be able to leverage it.
*/
ALTER TABLE "tenant"."users" DROP CONSTRAINT "pgschemadiff_tmpidx_users_uq_external_id_bpGqLi6qTPqFhOo7IGQNfw";

SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

CREATE SCHEMA "authn";

ALTER SCHEMA authn OWNER TO fun_owner;

GRANT USAGE
   ON SCHEMA authn
   TO fun_authn_api;

GRANT USAGE
   ON SCHEMA authn
   TO fun_fundament_api;

CREATE TABLE "authn"."api_keys" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"organization_id" uuid NOT NULL,
	"user_id" uuid NOT NULL,
	"name" text COLLATE "pg_catalog"."default" NOT NULL,
	"token_hash" bytea NOT NULL,
	"token_prefix" text COLLATE "pg_catalog"."default" NOT NULL,
	"expires" timestamp with time zone,
  "revoked" timestamp with time zone,
	"last_used" timestamp with time zone,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

ALTER TABLE authn.api_keys OWNER TO fun_owner;

CREATE POLICY "api_keys_organization_policy" ON "authn"."api_keys"
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING (((organization_id = (current_setting('app.current_organization_id'::text))::uuid) AND (user_id = (current_setting('app.current_user_id'::text))::uuid)));

ALTER TABLE "authn"."api_keys" ENABLE ROW LEVEL SECURITY;

CREATE UNIQUE INDEX api_keys_pk ON authn.api_keys USING btree (id);

ALTER TABLE "authn"."api_keys" ADD CONSTRAINT "api_keys_pk" PRIMARY KEY USING INDEX "api_keys_pk";

CREATE UNIQUE INDEX api_keys_uq_name ON authn.api_keys USING btree (organization_id, name, deleted) NULLS NOT DISTINCT;

ALTER TABLE "authn"."api_keys" ADD CONSTRAINT "api_keys_uq_name" UNIQUE USING INDEX "api_keys_uq_name";

CREATE UNIQUE INDEX api_keys_uq_token_hash ON authn.api_keys USING btree (token_hash);

ALTER TABLE "authn"."api_keys" ADD CONSTRAINT "api_keys_uq_token_hash" UNIQUE USING INDEX "api_keys_uq_token_hash";

ALTER TABLE "authn"."api_keys" ADD CONSTRAINT "api_keys_fk_organization" FOREIGN KEY (organization_id) REFERENCES tenant.organizations(id) NOT VALID;

ALTER TABLE "authn"."api_keys" VALIDATE CONSTRAINT "api_keys_fk_organization";

ALTER TABLE "authn"."api_keys" ADD CONSTRAINT "api_keys_fk_user" FOREIGN KEY (user_id) REFERENCES tenant.users(id) NOT VALID;

ALTER TABLE "authn"."api_keys" VALIDATE CONSTRAINT "api_keys_fk_user";

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authn.api_key_get_by_hash(p_token_hash bytea)
 RETURNS authn.api_keys
 LANGUAGE plpgsql
 SECURITY DEFINER COST 10
AS $function$
DECLARE
	result authn.api_keys;
	key_record authn.api_keys;
BEGIN
	SELECT * INTO key_record FROM authn.api_keys WHERE token_hash = p_token_hash;

	IF NOT FOUND THEN
		RETURN NULL;
	END IF;

	IF key_record.deleted IS NOT NULL THEN
		RAISE EXCEPTION 'API key has been deleted' USING HINT = 'api_key_deleted';
	END IF;

	IF key_record.revoked IS NOT NULL THEN
		RAISE EXCEPTION 'API key has been revoked' USING HINT = 'api_key_revoked';
	END IF;

	IF key_record.expires IS NOT NULL AND key_record.expires <= NOW() THEN
		RAISE EXCEPTION 'API key has expired' USING HINT = 'api_key_expired';
	END IF;

	UPDATE authn.api_keys
	SET last_used = NOW()
	WHERE id = key_record.id
	RETURNING * INTO result;

	RETURN result;
END;
$function$
;




-- Statements generated automatically, please review:
ALTER FUNCTION authn.api_key_get_by_hash(p_token_hash bytea) OWNER TO fun_owner;

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "authn"."api_keys" TO "fun_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "authn"."api_keys" TO "fun_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "authn"."api_keys" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "authn"."api_keys" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "authn"."api_keys" TO "fun_fundament_api";

SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

CREATE OR REPLACE FUNCTION authn.current_organization_id()
 RETURNS uuid
 LANGUAGE sql
 STABLE PARALLEL SAFE COST 1
AS $function$
SELECT current_setting('app.current_organization_id')::uuid
$function$
;

CREATE OR REPLACE FUNCTION authn.current_user_id()
 RETURNS uuid
 LANGUAGE sql
 STABLE PARALLEL SAFE COST 1
AS $function$
SELECT current_setting('app.current_user_id')::uuid
$function$
;

CREATE OR REPLACE FUNCTION authn.is_cluster_in_organization(p_cluster_id uuid)
 RETURNS boolean
 LANGUAGE sql
 STABLE PARALLEL SAFE SECURITY DEFINER COST 1
AS $function$
SELECT EXISTS (
    SELECT 1 FROM tenant.clusters
    WHERE id = p_cluster_id
    AND organization_id = authn.current_organization_id()
)
$function$
;

ALTER FUNCTION authn.is_cluster_in_organization(p_cluster_id uuid) OWNER TO fun_authz;

CREATE OR REPLACE FUNCTION authn.is_project_in_organization(p_project_id uuid)
 RETURNS boolean
 LANGUAGE sql
 STABLE PARALLEL SAFE SECURITY DEFINER COST 1
AS $function$
SELECT EXISTS (
    SELECT 1 FROM tenant.projects
    WHERE id = p_project_id
    AND organization_id = authn.current_organization_id()
)
$function$
;

ALTER FUNCTION authn.is_project_in_organization(p_project_id uuid) OWNER TO fun_authz;

CREATE OR REPLACE FUNCTION authn.is_user_in_organization(p_user_id uuid)
 RETURNS boolean
 LANGUAGE sql
 STABLE PARALLEL SAFE SECURITY DEFINER COST 1
AS $function$
SELECT EXISTS (
    SELECT 1 FROM tenant.users
    WHERE id = p_user_id
    AND organization_id = authn.current_organization_id()
)
$function$
;

ALTER FUNCTION authn.is_user_in_organization(p_user_id uuid) OWNER TO fun_authz;

/* Hazards:
 - AUTHZ_UPDATE: Altering a policy could cause queries to fail if not correctly configured or allow unauthorized access to data.
*/
ALTER POLICY "organization_isolation" ON "tenant"."clusters"
	USING ((organization_id = authn.current_organization_id()));

/* Hazards:
 - AUTHZ_UPDATE: Altering a policy could cause queries to fail if not correctly configured or allow unauthorized access to data.
*/
ALTER POLICY "namespaces_organization_policy" ON "tenant"."namespaces"
	USING (authn.is_cluster_in_organization(cluster_id));

/* Hazards:
 - AUTHZ_UPDATE: Altering a policy could cause queries to fail if not correctly configured or allow unauthorized access to data.
*/
ALTER POLICY "node_pools_organization_policy" ON "tenant"."node_pools"
	USING (authn.is_cluster_in_organization(cluster_id));

CREATE TABLE "tenant"."project_members" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"project_id" uuid NOT NULL,
	"user_id" uuid NOT NULL,
	"role" text COLLATE "pg_catalog"."default" NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

ALTER TABLE tenant.project_members OWNER TO fun_owner;

ALTER TABLE "tenant"."project_members" ADD CONSTRAINT "project_members_ck_role" CHECK((role = ANY (ARRAY['admin'::text, 'viewer'::text])));

CREATE OR REPLACE FUNCTION authn.is_project_member(p_project_id uuid, p_user_id uuid, p_role text)
 RETURNS boolean
 LANGUAGE sql
 STABLE PARALLEL SAFE SECURITY DEFINER COST 1
AS $function$
SELECT EXISTS (
    SELECT 1 FROM tenant.project_members
    WHERE project_id = p_project_id
    AND user_id = p_user_id
    AND (p_role IS NULL OR role = p_role)
    AND deleted IS NULL
)
$function$
;

ALTER FUNCTION authn.is_project_member(p_project_id uuid, p_user_id uuid, p_role text) OWNER TO fun_authz;


CREATE OR REPLACE FUNCTION tenant.project_has_members(p_project_id uuid)
 RETURNS boolean
 LANGUAGE sql
 STABLE PARALLEL SAFE SECURITY DEFINER COST 1
AS $function$
SELECT EXISTS (
    SELECT 1 FROM tenant.project_members
    WHERE project_id = p_project_id
    AND deleted IS NULL
)
$function$
;

ALTER FUNCTION tenant.project_has_members(p_project_id uuid) OWNER TO fun_authz;

CREATE POLICY "project_members_select_policy" ON "tenant"."project_members"
	AS PERMISSIVE
	FOR SELECT
	TO fun_fundament_api
    USING ((authn.is_project_in_organization(project_id) AND (authn.is_project_member(project_id, authn.current_user_id(), NULL::text) OR (user_id = authn.current_user_id()))));

CREATE POLICY "project_members_insert_policy" ON "tenant"."project_members"
    AS PERMISSIVE
    FOR INSERT
    TO fun_fundament_api
    WITH CHECK ((authn.is_project_in_organization(project_id) AND ((deleted IS NOT NULL) OR authn.is_user_in_organization(user_id)) AND (authn.is_project_member(project_id, authn.current_user_id(), 'admin'::text) OR (NOT tenant.project_has_members(project_id)))));

CREATE POLICY "project_members_update_policy" ON "tenant"."project_members"
    AS PERMISSIVE
    FOR UPDATE
    TO fun_fundament_api
    USING ((authn.is_project_in_organization(project_id) AND authn.is_project_member(project_id, authn.current_user_id(), 'admin'::text)));

ALTER TABLE "tenant"."project_members" ENABLE ROW LEVEL SECURITY;

CREATE UNIQUE INDEX project_members_pk ON tenant.project_members USING btree (id);

ALTER TABLE "tenant"."project_members" ADD CONSTRAINT "project_members_pk" PRIMARY KEY USING INDEX "project_members_pk";

CREATE UNIQUE INDEX project_members_uq_project_user ON tenant.project_members USING btree (project_id, user_id, deleted) NULLS NOT DISTINCT;

ALTER TABLE "tenant"."project_members" ADD CONSTRAINT "project_members_uq_project_user" UNIQUE USING INDEX "project_members_uq_project_user";

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.project_members_tr_protect_last_admin()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
DECLARE
    admin_count integer;
BEGIN
    -- Only check if we're soft-deleting an admin or demoting an admin (UPDATE role from admin)
    IF OLD.role = 'admin' AND OLD.deleted IS NULL THEN
        -- Check if this is a soft delete (setting deleted) or role demotion
        IF (NEW.deleted IS NOT NULL) OR (NEW.role != 'admin') THEN
            SELECT COUNT(*) INTO admin_count
            FROM tenant.project_members
            WHERE project_id = OLD.project_id
            AND role = 'admin'
            AND id != OLD.id
            AND deleted IS NULL;

            IF admin_count = 0 THEN
                RAISE EXCEPTION 'Cannot remove or demote the last admin of a project'
                            USING HINT = 'project_contains_one_admin';
            END IF;
        END IF;
    END IF;

    RETURN NEW;
END;
$function$
;

CREATE OR REPLACE TRIGGER protect_last_admin BEFORE UPDATE ON tenant.project_members FOR EACH ROW EXECUTE FUNCTION tenant.project_members_tr_protect_last_admin();

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "projects_delete_policy" ON "tenant"."projects"
	AS PERMISSIVE
	FOR DELETE
	TO fun_fundament_api
	USING (((organization_id = authn.current_organization_id()) AND authn.is_project_member(id, authn.current_user_id(), 'admin'::text)));

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "projects_insert_policy" ON "tenant"."projects"
	AS PERMISSIVE
	FOR INSERT
	TO fun_fundament_api
	WITH CHECK ((organization_id = authn.current_organization_id()));

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "projects_select_policy" ON "tenant"."projects"
	AS PERMISSIVE
	FOR SELECT
	TO fun_fundament_api
	USING ((organization_id = authn.current_organization_id()));

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "projects_update_policy" ON "tenant"."projects"
	AS PERMISSIVE
	FOR UPDATE
	TO fun_fundament_api
	USING (((organization_id = authn.current_organization_id()) AND authn.is_project_member(id, authn.current_user_id(), 'admin'::text)));

ALTER TABLE "tenant"."project_members" ADD CONSTRAINT "project_members_fk_project" FOREIGN KEY (project_id) REFERENCES tenant.projects(id) NOT VALID;

ALTER TABLE "tenant"."project_members" VALIDATE CONSTRAINT "project_members_fk_project";

ALTER TABLE "tenant"."project_members" ADD CONSTRAINT "project_members_fk_user" FOREIGN KEY (user_id) REFERENCES tenant.users(id) NOT VALID;

ALTER TABLE "tenant"."project_members" VALIDATE CONSTRAINT "project_members_fk_user";

/* Hazards:
 - AUTHZ_UPDATE: Altering a policy could cause queries to fail if not correctly configured or allow unauthorized access to data.
*/
ALTER POLICY "install_organization_policy" ON "appstore"."installs"
	USING (authn.is_cluster_in_organization(cluster_id));


-- Statements generated automatically, please review:
ALTER FUNCTION authn.current_organization_id() OWNER TO fun_fundament_api;
ALTER FUNCTION authn.current_user_id() OWNER TO fun_fundament_api;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.projects_tr_require_admin()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM tenant.project_members
        WHERE project_id = NEW.id
        AND role = 'admin'
        AND deleted IS NULL
    ) THEN
        RAISE EXCEPTION 'Project must have at least one admin';
    END IF;
    RETURN NEW;
END;
$function$
;

CREATE CONSTRAINT TRIGGER require_admin AFTER INSERT ON tenant.projects DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION tenant.projects_tr_require_admin();

CREATE INDEX project_members_idx_project_id ON tenant.project_members USING btree (project_id);

/* Hazards:
 - AUTHZ_UPDATE: Removing a permissive policy could cause queries to fail if not correctly configured.
*/
DROP POLICY "projects_organization_isolation" ON "tenant"."projects";

GRANT USAGE
   ON SCHEMA tenant
   TO fun_authz;

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."clusters" TO "fun_authz";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."project_members" TO "fun_authz";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."projects" TO "fun_authz";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."users" TO "fun_authz";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "tenant"."project_members" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."project_members" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "tenant"."project_members" TO "fun_fundament_api";

SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

GRANT USAGE
   ON SCHEMA authn
   TO fun_authz;

SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

CREATE SCHEMA "authz";

GRANT USAGE
   ON SCHEMA authz
   TO fun_fundament_api;

GRANT USAGE
   ON SCHEMA authz
   TO fun_authn_api;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.api_keys_sync_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    INSERT INTO authz.outbox (api_key_id)
    VALUES (COALESCE(NEW.id, OLD.id));
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.clusters_sync_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    INSERT INTO authz.outbox (cluster_id)
    VALUES (COALESCE(NEW.id, OLD.id));
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.installs_sync_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    INSERT INTO authz.outbox (install_id)
    VALUES (COALESCE(NEW.id, OLD.id));
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.namespaces_sync_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    INSERT INTO authz.outbox (namespace_id)
    VALUES (COALESCE(NEW.id, OLD.id));
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.node_pools_sync_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    INSERT INTO authz.outbox (node_pool_id)
    VALUES (COALESCE(NEW.id, OLD.id));
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.outbox_notify_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    PERFORM pg_notify('authz_outbox', '');
    RETURN NEW;
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.project_members_sync_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    INSERT INTO authz.outbox (project_member_id)
    VALUES (COALESCE(NEW.id, OLD.id));
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.projects_sync_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    INSERT INTO authz.outbox (project_id)
    VALUES (COALESCE(NEW.id, OLD.id));
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.users_sync_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    INSERT INTO authz.outbox (user_id)
    VALUES (COALESCE(NEW.id, OLD.id));
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

ALTER INDEX "tenant"."organizations_uq_name" RENAME TO "pgschemadiff_tmpidx_organizations_uq_nam_75SClg18S2mT9vWHkD9SFw";

CREATE TABLE "authz"."outbox" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"user_id" uuid,
	"project_id" uuid,
	"project_member_id" uuid,
	"cluster_id" uuid,
	"node_pool_id" uuid,
	"namespace_id" uuid,
	"api_key_id" uuid,
	"install_id" uuid,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"processed" timestamp with time zone,
	"retries" integer DEFAULT 0 NOT NULL,
	"retry_after" timestamp with time zone,
	"failed" timestamp with time zone,
	status text NOT NULL DEFAULT 'pending',
	status_info text
);

ALTER TABLE "authz"."outbox" ADD CONSTRAINT "outbox_ck_single_fk" CHECK((num_nonnulls(user_id, project_id, project_member_id, cluster_id, node_pool_id, namespace_id, api_key_id, install_id) = 1));

ALTER TABLE "authz"."outbox" ADD CONSTRAINT "outbox_ck_status" CHECK((status = ANY (ARRAY['pending'::text, 'completed'::text, 'retrying'::text, 'failed'::text]))) NOT VALID;

ALTER TABLE "authz"."outbox" VALIDATE CONSTRAINT "outbox_ck_status";

GRANT INSERT ON "authz"."outbox" TO "fun_authn_api";

GRANT INSERT ON "authz"."outbox" TO "fun_fundament_api";

ALTER TABLE "authz"."outbox" ADD CONSTRAINT "outbox_fk_api_key" FOREIGN KEY (api_key_id) REFERENCES authn.api_keys(id) NOT VALID;

ALTER TABLE "authz"."outbox" VALIDATE CONSTRAINT "outbox_fk_api_key";

CREATE TRIGGER api_keys_outbox AFTER INSERT OR UPDATE ON authn.api_keys FOR EACH ROW EXECUTE FUNCTION authz.api_keys_sync_trigger();

CREATE UNIQUE INDEX outbox_pk ON authz.outbox USING btree (id);

ALTER TABLE "authz"."outbox" ADD CONSTRAINT "outbox_pk" PRIMARY KEY USING INDEX "outbox_pk";

CREATE INDEX outbox_idx_unprocessed ON authz.outbox USING btree (created) WHERE (processed IS NULL);

CREATE TRIGGER outbox_notify AFTER INSERT ON authz.outbox FOR EACH ROW EXECUTE FUNCTION authz.outbox_notify_trigger();

CREATE TRIGGER clusters_outbox AFTER INSERT OR UPDATE ON tenant.clusters FOR EACH ROW EXECUTE FUNCTION authz.clusters_sync_trigger();

ALTER TABLE "authz"."outbox" ADD CONSTRAINT "outbox_fk_cluster" FOREIGN KEY (cluster_id) REFERENCES tenant.clusters(id) NOT VALID;

ALTER TABLE "authz"."outbox" VALIDATE CONSTRAINT "outbox_fk_cluster";

CREATE TRIGGER namespaces_outbox AFTER INSERT OR UPDATE ON tenant.namespaces FOR EACH ROW EXECUTE FUNCTION authz.namespaces_sync_trigger();

ALTER TABLE "authz"."outbox" ADD CONSTRAINT "outbox_fk_namespace" FOREIGN KEY (namespace_id) REFERENCES tenant.namespaces(id) NOT VALID;

ALTER TABLE "authz"."outbox" VALIDATE CONSTRAINT "outbox_fk_namespace";

CREATE TRIGGER node_pools_outbox AFTER INSERT OR UPDATE ON tenant.node_pools FOR EACH ROW EXECUTE FUNCTION authz.node_pools_sync_trigger();

ALTER TABLE "authz"."outbox" ADD CONSTRAINT "outbox_fk_node_pool" FOREIGN KEY (node_pool_id) REFERENCES tenant.node_pools(id) NOT VALID;

ALTER TABLE "authz"."outbox" VALIDATE CONSTRAINT "outbox_fk_node_pool";

ALTER TABLE "tenant"."organizations" ADD COLUMN "deleted" timestamp with time zone;

/* Hazards:
 - ACQUIRES_SHARE_LOCK: Non-concurrent index creates will lock out writes to the table during the duration of the index build.
*/
CREATE UNIQUE INDEX organizations_uq_name ON tenant.organizations USING btree (name, deleted) NULLS NOT DISTINCT;

ALTER TABLE "tenant"."organizations" ADD CONSTRAINT "organizations_uq_name" UNIQUE USING INDEX "organizations_uq_name";

CREATE TRIGGER project_members_outbox AFTER INSERT OR UPDATE ON tenant.project_members FOR EACH ROW EXECUTE FUNCTION authz.project_members_sync_trigger();

ALTER TABLE "authz"."outbox" ADD CONSTRAINT "outbox_fk_project_member" FOREIGN KEY (project_member_id) REFERENCES tenant.project_members(id) NOT VALID;

ALTER TABLE "authz"."outbox" VALIDATE CONSTRAINT "outbox_fk_project_member";

CREATE TRIGGER projects_outbox AFTER INSERT OR UPDATE ON tenant.projects FOR EACH ROW EXECUTE FUNCTION authz.projects_sync_trigger();

ALTER TABLE "authz"."outbox" ADD CONSTRAINT "outbox_fk_project" FOREIGN KEY (project_id) REFERENCES tenant.projects(id) NOT VALID;

ALTER TABLE "authz"."outbox" VALIDATE CONSTRAINT "outbox_fk_project";

CREATE TRIGGER users_outbox AFTER INSERT OR UPDATE ON tenant.users FOR EACH ROW EXECUTE FUNCTION authz.users_sync_trigger();

ALTER TABLE "authz"."outbox" ADD CONSTRAINT "outbox_fk_user" FOREIGN KEY (user_id) REFERENCES tenant.users(id) NOT VALID;

ALTER TABLE "authz"."outbox" VALIDATE CONSTRAINT "outbox_fk_user";

CREATE TRIGGER installs_outbox AFTER INSERT OR UPDATE ON appstore.installs FOR EACH ROW EXECUTE FUNCTION authz.installs_sync_trigger();

ALTER TABLE "authz"."outbox" ADD CONSTRAINT "outbox_fk_install" FOREIGN KEY (install_id) REFERENCES appstore.installs(id) NOT VALID;

ALTER TABLE "authz"."outbox" VALIDATE CONSTRAINT "outbox_fk_install";

/* Hazards:
 - ACQUIRES_ACCESS_EXCLUSIVE_LOCK: Index drops will lock out all accesses to the table. They should be fast.
 - INDEX_DROPPED: Dropping this index means queries that use this index might perform worse because they will no longer will be able to leverage it.
*/
ALTER TABLE "tenant"."organizations" DROP CONSTRAINT "pgschemadiff_tmpidx_organizations_uq_nam_75SClg18S2mT9vWHkD9SFw";


-- Statements generated automatically, please review:
ALTER SCHEMA authz OWNER TO fun_owner;
ALTER FUNCTION authz.api_keys_sync_trigger() OWNER TO fun_owner;
ALTER FUNCTION authz.clusters_sync_trigger() OWNER TO fun_owner;
ALTER FUNCTION authz.installs_sync_trigger() OWNER TO fun_owner;
ALTER FUNCTION authz.namespaces_sync_trigger() OWNER TO fun_owner;
ALTER FUNCTION authz.node_pools_sync_trigger() OWNER TO fun_owner;
ALTER FUNCTION authz.outbox_notify_trigger() OWNER TO fun_owner;
ALTER FUNCTION authz.project_members_sync_trigger() OWNER TO fun_owner;
ALTER FUNCTION authz.projects_sync_trigger() OWNER TO fun_owner;
ALTER FUNCTION authz.users_sync_trigger() OWNER TO fun_owner;
ALTER TABLE authz.outbox OWNER TO fun_owner;

-- Grants for fun_authz_worker to process the outbox
GRANT USAGE ON SCHEMA authz TO fun_authz_worker;
GRANT USAGE ON SCHEMA tenant TO fun_authz_worker;
GRANT USAGE ON SCHEMA authn TO fun_authz_worker;
GRANT USAGE ON SCHEMA appstore TO fun_authz_worker;

GRANT SELECT ON public.schema_migrations TO fun_authz_worker;

GRANT SELECT, UPDATE ON authz.outbox TO fun_authz_worker;

GRANT SELECT ON tenant.users TO fun_authz_worker;
GRANT SELECT ON tenant.projects TO fun_authz_worker;
GRANT SELECT ON tenant.project_members TO fun_authz_worker;
GRANT SELECT ON tenant.clusters TO fun_authz_worker;
GRANT SELECT ON tenant.node_pools TO fun_authz_worker;
GRANT SELECT ON tenant.namespaces TO fun_authz_worker;
GRANT SELECT ON tenant.organizations TO fun_authz_worker;

GRANT SELECT ON authn.api_keys TO fun_authz_worker;

GRANT SELECT ON appstore.installs TO fun_authz_worker;

SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.cluster_reset_synced()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    NEW.synced := NULL;
    NEW.sync_claimed_at := NULL;
    NEW.sync_attempts := 0;
    NEW.sync_error := NULL;
    RETURN NEW;
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.cluster_sync_notify()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    IF NEW.synced IS NULL AND (TG_OP = 'INSERT' OR OLD.synced IS NOT NULL) THEN
        PERFORM pg_notify('cluster_sync', '');
    END IF;
    RETURN NEW;
END;
$function$
;

-- WARNING: The following grants were added manually because trek does not
-- generate USAGE grants or cross-schema permissions in migration diffs.
GRANT USAGE ON SCHEMA "tenant" TO "fun_cluster_worker";
GRANT USAGE ON SCHEMA "authz" TO "fun_cluster_worker";
GRANT INSERT ON "authz"."outbox" TO "fun_cluster_worker";
GRANT SELECT ON "tenant"."namespaces" TO "fun_cluster_worker";

CREATE TABLE "tenant"."cluster_events" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"cluster_id" uuid NOT NULL,
	"event_type" text COLLATE "pg_catalog"."default" NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"sync_action" text COLLATE "pg_catalog"."default",
	"message" text COLLATE "pg_catalog"."default",
	"attempt" integer
);

ALTER TABLE "tenant"."cluster_events" ADD CONSTRAINT "cluster_events_ck_event_type" CHECK((event_type = ANY (ARRAY['sync_requested'::text, 'sync_claimed'::text, 'sync_succeeded'::text, 'sync_failed'::text, 'status_progressing'::text, 'status_ready'::text, 'status_error'::text, 'status_deleted'::text])));

ALTER TABLE "tenant"."cluster_events" ADD CONSTRAINT "cluster_events_ck_sync_action" CHECK((sync_action = ANY (ARRAY['sync'::text, 'delete'::text])));

CREATE POLICY "cluster_events_organization_isolation" ON "tenant"."cluster_events"
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING ((EXISTS ( SELECT 1
   FROM tenant.clusters c
  WHERE ((c.id = cluster_events.cluster_id) AND (c.organization_id = authn.current_organization_id())))));

CREATE POLICY "cluster_events_worker_all_access" ON "tenant"."cluster_events"
	AS PERMISSIVE
	FOR ALL
	TO fun_cluster_worker
	USING (true);

ALTER TABLE "tenant"."cluster_events" ENABLE ROW LEVEL SECURITY;

GRANT INSERT ON "tenant"."cluster_events" TO "fun_cluster_worker";

GRANT SELECT ON "tenant"."cluster_events" TO "fun_cluster_worker";

GRANT INSERT ON "tenant"."cluster_events" TO "fun_fundament_api";

GRANT SELECT ON "tenant"."cluster_events" TO "fun_fundament_api";

CREATE UNIQUE INDEX cluster_events_pk ON tenant.cluster_events USING btree (id);

ALTER TABLE "tenant"."cluster_events" ADD CONSTRAINT "cluster_events_pk" PRIMARY KEY USING INDEX "cluster_events_pk";

CREATE INDEX cluster_events_idx_cluster_created ON tenant.cluster_events USING btree (cluster_id DESC NULLS LAST, created DESC NULLS LAST);

ALTER TABLE "tenant"."clusters" ADD COLUMN "shoot_status" text COLLATE "pg_catalog"."default";

ALTER TABLE "tenant"."clusters" ADD COLUMN "shoot_status_message" text COLLATE "pg_catalog"."default";

ALTER TABLE "tenant"."clusters" ADD COLUMN "shoot_status_updated" timestamp with time zone;

ALTER TABLE "tenant"."clusters" ADD COLUMN "sync_attempts" integer DEFAULT 0 NOT NULL;

ALTER TABLE "tenant"."clusters" ADD COLUMN "sync_claimed_at" timestamp with time zone;

ALTER TABLE "tenant"."clusters" ADD COLUMN "sync_error" text COLLATE "pg_catalog"."default";

ALTER TABLE "tenant"."clusters" ADD COLUMN "synced" timestamp with time zone;

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "cluster_worker_all_access" ON "tenant"."clusters"
	AS PERMISSIVE
	FOR ALL
	TO fun_cluster_worker
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."clusters" TO "fun_cluster_worker";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "tenant"."clusters" TO "fun_cluster_worker";

ALTER TABLE "tenant"."clusters" DROP CONSTRAINT "clusters_ck_status";

/* Hazards:
 - DELETES_DATA: Deletes all values in the column
*/
ALTER TABLE "tenant"."clusters" DROP COLUMN "status";

/* Hazards:
 - ACQUIRES_SHARE_LOCK: Non-concurrent index creates will lock out writes to the table during the duration of the index build.
*/
CREATE INDEX clusters_idx_needs_sync ON tenant.clusters USING btree (created) WHERE (synced IS NULL);

CREATE TRIGGER cluster_reset_synced BEFORE UPDATE OF name, region, kubernetes_version, deleted ON tenant.clusters FOR EACH ROW WHEN (((old.name IS DISTINCT FROM new.name) OR (old.region IS DISTINCT FROM new.region) OR (old.kubernetes_version IS DISTINCT FROM new.kubernetes_version) OR ((old.deleted IS NULL) AND (new.deleted IS NOT NULL)))) EXECUTE FUNCTION tenant.cluster_reset_synced();

CREATE TRIGGER cluster_sync_notify AFTER INSERT OR UPDATE ON tenant.clusters FOR EACH ROW EXECUTE FUNCTION tenant.cluster_sync_notify();

ALTER TABLE "tenant"."cluster_events" ADD CONSTRAINT "cluster_events_fk_cluster" FOREIGN KEY (cluster_id) REFERENCES tenant.clusters(id) ON DELETE CASCADE NOT VALID;

ALTER TABLE "tenant"."cluster_events" VALIDATE CONSTRAINT "cluster_events_fk_cluster";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."organizations" TO "fun_cluster_worker";


-- Statements generated automatically, please review:
ALTER FUNCTION tenant.cluster_reset_synced() OWNER TO fun_owner;
ALTER FUNCTION tenant.cluster_sync_notify() OWNER TO fun_owner;
ALTER TABLE tenant.cluster_events OWNER TO fun_owner;

SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - ACQUIRES_SHARE_LOCK: Non-concurrent index creates will lock out writes to the table during the duration of the index build.
*/
CREATE INDEX namespaces_ix_cluster_name ON tenant.namespaces USING btree (cluster_id, name) WHERE (deleted IS NULL);

SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.api_keys_sync_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    -- Only insert into outbox if this is an INSERT, DELETE, or if data actually changed
    IF TG_OP = 'INSERT' OR NEW IS DISTINCT FROM OLD THEN
        INSERT INTO authz.outbox (api_key_id)
        VALUES (COALESCE(NEW.id, OLD.id));
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.clusters_sync_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    -- Only insert into outbox if this is an INSERT or if data actually changed
    IF TG_OP = 'INSERT' OR NEW IS DISTINCT FROM OLD THEN
        INSERT INTO authz.outbox (cluster_id)
        VALUES (COALESCE(NEW.id, OLD.id));
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.installs_sync_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    -- Only insert into outbox if this is an INSERT or if data actually changed
    IF TG_OP = 'INSERT' OR NEW IS DISTINCT FROM OLD THEN
        INSERT INTO authz.outbox (install_id)
        VALUES (COALESCE(NEW.id, OLD.id));
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.namespaces_sync_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    -- Only insert into outbox if this is an INSERT or if data actually changed
    IF TG_OP = 'INSERT' OR NEW IS DISTINCT FROM OLD THEN
        INSERT INTO authz.outbox (namespace_id)
        VALUES (COALESCE(NEW.id, OLD.id));
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.node_pools_sync_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    -- Only insert into outbox if this is an INSERT or if data actually changed
    IF TG_OP = 'INSERT' OR NEW IS DISTINCT FROM OLD THEN
        INSERT INTO authz.outbox (node_pool_id)
        VALUES (COALESCE(NEW.id, OLD.id));
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.project_members_sync_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    -- Only insert into outbox if this is an INSERT or if data actually changed
    IF TG_OP = 'INSERT' OR NEW IS DISTINCT FROM OLD THEN
        INSERT INTO authz.outbox (project_member_id)
        VALUES (COALESCE(NEW.id, OLD.id));
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.projects_sync_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    -- Only insert into outbox if this is an INSERT or if data actually changed
    IF TG_OP = 'INSERT' OR NEW IS DISTINCT FROM OLD THEN
        INSERT INTO authz.outbox (project_id)
        VALUES (COALESCE(NEW.id, OLD.id));
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.users_sync_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    -- Only insert into outbox if this is an INSERT or if data actually changed
    IF TG_OP = 'INSERT' OR NEW IS DISTINCT FROM OLD THEN
        INSERT INTO authz.outbox (user_id)
        VALUES (COALESCE(NEW.id, OLD.id));
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

ALTER TABLE "tenant"."users" ADD CONSTRAINT "users_ck_role" CHECK((role = ANY (ARRAY['admin'::text, 'viewer'::text]))) NOT VALID;

ALTER TABLE "tenant"."users" VALIDATE CONSTRAINT "users_ck_role";

