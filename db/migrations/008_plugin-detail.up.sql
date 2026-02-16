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
