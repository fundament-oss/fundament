SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

CREATE TABLE "appstore"."plugin_definitions" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"plugin_id" uuid NOT NULL,
	"plugin_version" text COLLATE "pg_catalog"."default" NOT NULL,
	"manifest" bytea NOT NULL,
	"hash" text COLLATE "pg_catalog"."default" NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

GRANT INSERT ON "appstore"."plugin_definitions" TO "fun_fundament_api";

GRANT SELECT ON "appstore"."plugin_definitions" TO "fun_fundament_api";

GRANT UPDATE ON "appstore"."plugin_definitions" TO "fun_fundament_api";

CREATE UNIQUE INDEX plugin_definitions_pk ON appstore.plugin_definitions USING btree (id);

ALTER TABLE "appstore"."plugin_definitions" ADD CONSTRAINT "plugin_definitions_pk" PRIMARY KEY USING INDEX "plugin_definitions_pk";

CREATE UNIQUE INDEX plugin_definitions_uq_plugin_version ON appstore.plugin_definitions USING btree (plugin_id, plugin_version, deleted) NULLS NOT DISTINCT;

ALTER TABLE "appstore"."plugin_definitions" ADD CONSTRAINT "plugin_definitions_uq_plugin_version" UNIQUE USING INDEX "plugin_definitions_uq_plugin_version";

ALTER TABLE "appstore"."plugin_definitions" ADD CONSTRAINT "plugin_definitions_fk_plugin" FOREIGN KEY (plugin_id) REFERENCES appstore.plugins(id) NOT VALID;

ALTER TABLE "appstore"."plugin_definitions" VALIDATE CONSTRAINT "plugin_definitions_fk_plugin";


-- Statements generated automatically, please review:
ALTER TABLE appstore.plugin_definitions OWNER TO fun_owner;
