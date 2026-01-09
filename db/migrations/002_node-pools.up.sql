SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.cluster_node_pool_counter()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
      IF TG_OP = 'INSERT' THEN
            UPDATE tenant.clusters
            SET node_pool_count = node_pool_count + 1
            WHERE id = NEW.cluster_id;
            RETURN NEW;
      ELSIF TG_OP = 'UPDATE' THEN
            IF OLD.deleted IS NULL AND NEW.deleted IS NOT NULL THEN
                  UPDATE tenant.clusters
                  SET node_pool_count = node_pool_count  - 1
                  WHERE id = NEW.cluster_id;
            END IF;
            RETURN NEW;
      END IF;
      RETURN NULL;
END
$function$
;

ALTER TABLE "tenant"."clusters" ADD COLUMN "node_pool_count" integer DEFAULT 0 NOT NULL;

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

CREATE UNIQUE INDEX node_pools_uq_name ON tenant.node_pools USING btree (cluster_id, name, deleted);

ALTER TABLE "tenant"."node_pools" ADD CONSTRAINT "node_pools_uq_name" UNIQUE USING INDEX "node_pools_uq_name";

CREATE TRIGGER node_pool_count AFTER INSERT OR UPDATE OF deleted ON tenant.node_pools FOR EACH ROW EXECUTE FUNCTION tenant.cluster_node_pool_counter();

