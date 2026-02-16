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
