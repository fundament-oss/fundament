SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

CREATE TABLE "tenant"."plugins" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"cluster_id" uuid NOT NULL,
	"plugin_id" text COLLATE "pg_catalog"."default" NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

ALTER TABLE "tenant"."plugins" ENABLE ROW LEVEL SECURITY;

ALTER TABLE "tenant"."plugins" ADD CONSTRAINT "plugins_fk_cluster" FOREIGN KEY (cluster_id) REFERENCES tenant.clusters(id) NOT VALID;

ALTER TABLE "tenant"."plugins" VALIDATE CONSTRAINT "plugins_fk_cluster";

CREATE UNIQUE INDEX plugins_pk ON tenant.plugins USING btree (id);

ALTER TABLE "tenant"."plugins" ADD CONSTRAINT "plugins_pk" PRIMARY KEY USING INDEX "plugins_pk";

CREATE UNIQUE INDEX plugins_uq ON tenant.plugins USING btree (cluster_id, plugin_id, deleted) NULLS NOT DISTINCT;

ALTER TABLE "tenant"."plugins" ADD CONSTRAINT "plugins_uq" UNIQUE USING INDEX "plugins_uq";


-- Statements generated automatically, please review:
ALTER TABLE tenant.plugins OWNER TO fun_fundament_api;
