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

