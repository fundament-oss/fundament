SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

CREATE TABLE "tenant"."installs" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"cluster_id" uuid NOT NULL,
	"plugin_id" uuid NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

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
