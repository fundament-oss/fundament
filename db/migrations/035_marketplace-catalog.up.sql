SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

-- Schema USAGE for the catalog reader role. trek's diff emits the table SELECTs
-- below but not schema-level permissions, so these are hand-added and must be
-- re-added whenever this migration is regenerated. Without USAGE every SELECT
-- below is unusable.
GRANT USAGE ON SCHEMA "appstore" TO "fun_marketplace_catalog_api";
GRANT USAGE ON SCHEMA "tenant" TO "fun_marketplace_catalog_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."categories" TO "fun_marketplace_catalog_api";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "categories_plugins_all_api" ON "appstore"."categories_plugins"
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "categories_plugins_select_catalog" ON "appstore"."categories_plugins"
	AS PERMISSIVE
	FOR SELECT
	TO fun_marketplace_catalog_api
	USING ((EXISTS ( SELECT 1
   FROM appstore.plugins
  WHERE (plugins.id = categories_plugins.plugin_id))));

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."categories_plugins" TO "fun_marketplace_catalog_api";

/* Hazards:
 - AUTHZ_UPDATE: Enabling RLS on a table could cause queries to fail if not correctly configured.
*/
ALTER TABLE "appstore"."categories_plugins" ENABLE ROW LEVEL SECURITY;

ALTER TABLE "appstore"."plugin_definitions" ADD COLUMN "published" timestamp with time zone;

ALTER TABLE "appstore"."plugin_definitions" ADD COLUMN "release_notes" text COLLATE "pg_catalog"."default" DEFAULT ''::text NOT NULL;

ALTER TABLE "appstore"."plugin_definitions" ADD COLUMN "status" text COLLATE "pg_catalog"."default" DEFAULT 'draft'::text NOT NULL;

ALTER TABLE "appstore"."plugin_definitions" ADD CONSTRAINT "plugin_definitions_ck_status" CHECK((status = ANY (ARRAY['draft'::text, 'pending'::text, 'changes_requested'::text, 'approved'::text, 'rejected'::text, 'withdrawn'::text]))) NOT VALID;

ALTER TABLE "appstore"."plugin_definitions" VALIDATE CONSTRAINT "plugin_definitions_ck_status";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "plugin_definitions_select_catalog" ON "appstore"."plugin_definitions"
	AS PERMISSIVE
	FOR SELECT
	TO fun_marketplace_catalog_api
	USING (((deleted IS NULL) AND (published IS NOT NULL)));

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."plugin_definitions" TO "fun_marketplace_catalog_api";

/* Hazards:
 - ACQUIRES_SHARE_LOCK: Non-concurrent index creates will lock out writes to the table during the duration of the index build.
*/
CREATE INDEX plugin_definitions_idx_published ON appstore.plugin_definitions USING btree (plugin_id, published) WHERE ((published IS NOT NULL) AND (deleted IS NULL));

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "plugin_documentation_links_all_api" ON "appstore"."plugin_documentation_links"
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "plugin_documentation_links_select_catalog" ON "appstore"."plugin_documentation_links"
	AS PERMISSIVE
	FOR SELECT
	TO fun_marketplace_catalog_api
	USING ((EXISTS ( SELECT 1
   FROM appstore.plugins
  WHERE (plugins.id = plugin_documentation_links.plugin_id))));

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."plugin_documentation_links" TO "fun_marketplace_catalog_api";

/* Hazards:
 - AUTHZ_UPDATE: Enabling RLS on a table could cause queries to fail if not correctly configured.
*/
ALTER TABLE "appstore"."plugin_documentation_links" ENABLE ROW LEVEL SECURITY;

CREATE TABLE "appstore"."plugin_features" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"plugin_id" uuid NOT NULL,
	"title" text COLLATE "pg_catalog"."default" NOT NULL,
	"body" text COLLATE "pg_catalog"."default" NOT NULL,
	"position" integer DEFAULT 0 NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

CREATE POLICY "plugin_features_select_catalog" ON "appstore"."plugin_features"
	AS PERMISSIVE
	FOR SELECT
	TO fun_marketplace_catalog_api
	USING (((deleted IS NULL) AND (EXISTS ( SELECT 1
   FROM appstore.plugins
  WHERE (plugins.id = plugin_features.plugin_id)))));

ALTER TABLE "appstore"."plugin_features" ENABLE ROW LEVEL SECURITY;

GRANT SELECT ON "appstore"."plugin_features" TO "fun_marketplace_catalog_api";

CREATE UNIQUE INDEX plugin_features_pk ON appstore.plugin_features USING btree (id);

ALTER TABLE "appstore"."plugin_features" ADD CONSTRAINT "plugin_features_pk" PRIMARY KEY USING INDEX "plugin_features_pk";

CREATE TABLE "appstore"."plugin_labels" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"plugin_id" uuid NOT NULL,
	"name" text COLLATE "pg_catalog"."default" NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

ALTER TABLE "appstore"."plugin_labels" ADD CONSTRAINT "plugin_labels_ck_name" CHECK((name = ANY (ARRAY['core'::text, 'rijksoverheid'::text, 'support_9_to_17'::text])));

CREATE POLICY "plugin_labels_select_catalog" ON "appstore"."plugin_labels"
	AS PERMISSIVE
	FOR SELECT
	TO fun_marketplace_catalog_api
	USING (((deleted IS NULL) AND (EXISTS ( SELECT 1
   FROM appstore.plugins
  WHERE (plugins.id = plugin_labels.plugin_id)))));

ALTER TABLE "appstore"."plugin_labels" ENABLE ROW LEVEL SECURITY;

GRANT SELECT ON "appstore"."plugin_labels" TO "fun_marketplace_catalog_api";

CREATE UNIQUE INDEX plugin_labels_pk ON appstore.plugin_labels USING btree (id);

ALTER TABLE "appstore"."plugin_labels" ADD CONSTRAINT "plugin_labels_pk" PRIMARY KEY USING INDEX "plugin_labels_pk";

CREATE UNIQUE INDEX plugin_labels_uq_name ON appstore.plugin_labels USING btree (plugin_id, name, deleted) NULLS NOT DISTINCT;

ALTER TABLE "appstore"."plugin_labels" ADD CONSTRAINT "plugin_labels_uq_name" UNIQUE USING INDEX "plugin_labels_uq_name";

ALTER TABLE "appstore"."plugins" ADD COLUMN "license" text COLLATE "pg_catalog"."default" DEFAULT ''::text NOT NULL;

ALTER TABLE "appstore"."plugins" ADD COLUMN "visibility" text COLLATE "pg_catalog"."default" DEFAULT 'public'::text NOT NULL;

ALTER TABLE "appstore"."plugins" ADD CONSTRAINT "plugins_ck_visibility" CHECK((visibility = ANY (ARRAY['public'::text, 'restricted'::text]))) NOT VALID;

ALTER TABLE "appstore"."plugins" VALIDATE CONSTRAINT "plugins_ck_visibility";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "plugins_select_catalog" ON "appstore"."plugins"
	AS PERMISSIVE
	FOR SELECT
	TO fun_marketplace_catalog_api
	USING (((deleted IS NULL) AND (visibility = 'public'::text) AND (EXISTS ( SELECT 1
   FROM appstore.plugin_definitions
  WHERE ((plugin_definitions.plugin_id = plugins.id) AND (plugin_definitions.published IS NOT NULL) AND (plugin_definitions.deleted IS NULL))))));

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."plugins" TO "fun_marketplace_catalog_api";

ALTER TABLE "appstore"."plugin_features" ADD CONSTRAINT "plugin_features_fk_plugin" FOREIGN KEY (plugin_id) REFERENCES appstore.plugins(id) NOT VALID;

ALTER TABLE "appstore"."plugin_features" VALIDATE CONSTRAINT "plugin_features_fk_plugin";

ALTER TABLE "appstore"."plugin_labels" ADD CONSTRAINT "plugin_labels_fk_plugin" FOREIGN KEY (plugin_id) REFERENCES appstore.plugins(id) NOT VALID;

ALTER TABLE "appstore"."plugin_labels" VALIDATE CONSTRAINT "plugin_labels_fk_plugin";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "plugins_tags_all_api" ON "appstore"."plugins_tags"
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "plugins_tags_select_catalog" ON "appstore"."plugins_tags"
	AS PERMISSIVE
	FOR SELECT
	TO fun_marketplace_catalog_api
	USING ((EXISTS ( SELECT 1
   FROM appstore.plugins
  WHERE (plugins.id = plugins_tags.plugin_id))));

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."plugins_tags" TO "fun_marketplace_catalog_api";

/* Hazards:
 - AUTHZ_UPDATE: Enabling RLS on a table could cause queries to fail if not correctly configured.
*/
ALTER TABLE "appstore"."plugins_tags" ENABLE ROW LEVEL SECURITY;

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."tags" TO "fun_marketplace_catalog_api";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "organizations_select_catalog" ON "tenant"."organizations"
	AS PERMISSIVE
	FOR SELECT
	TO fun_marketplace_catalog_api
	USING (((deleted IS NULL) AND (EXISTS ( SELECT 1
   FROM appstore.plugins
  WHERE ((plugins.organization_id = organizations.id) AND (plugins.deleted IS NULL) AND (plugins.visibility = 'public'::text) AND (EXISTS ( SELECT 1
           FROM appstore.plugin_definitions
          WHERE ((plugin_definitions.plugin_id = plugins.id) AND (plugin_definitions.published IS NOT NULL) AND (plugin_definitions.deleted IS NULL)))))))));

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."organizations" TO "fun_marketplace_catalog_api";


-- Statements generated automatically, please review:
ALTER TABLE appstore.plugin_features OWNER TO fun_owner;
ALTER TABLE appstore.plugin_labels OWNER TO fun_owner;
