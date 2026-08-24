SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

CREATE SCHEMA "catalog";

-- Schema USAGE for the reader roles (trek's diff emits the table SELECTs below
-- but not schema-level permissions; without USAGE the SELECTs are unusable).
GRANT USAGE ON SCHEMA "catalog" TO "fun_authn_api";
GRANT USAGE ON SCHEMA "catalog" TO "fun_cluster_worker";
GRANT USAGE ON SCHEMA "catalog" TO "fun_fundament_api";

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.node_pool_region_match_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 SECURITY DEFINER COST 1
AS $function$
BEGIN
    IF NEW.region_machine_type_id IS NOT NULL THEN
        IF (SELECT region_id FROM catalog.region_machine_types WHERE id = NEW.region_machine_type_id)
           IS DISTINCT FROM
           (SELECT region_id FROM tenant.clusters WHERE id = NEW.cluster_id)
        THEN
            RAISE EXCEPTION 'node_pool region_machine_type region does not match cluster region'
                        USING HINT = 'node_pool_region_mismatch';
        END IF;
    END IF;
    RETURN NULL;
END;
$function$
;

CREATE TABLE "catalog"."kubernetes_versions" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"version" text COLLATE "pg_catalog"."default" NOT NULL
);

GRANT SELECT ON "catalog"."kubernetes_versions" TO "fun_authn_api";

GRANT SELECT ON "catalog"."kubernetes_versions" TO "fun_cluster_worker";

GRANT SELECT ON "catalog"."kubernetes_versions" TO "fun_fundament_api";

CREATE UNIQUE INDEX kubernetes_versions_pk ON catalog.kubernetes_versions USING btree (id);

ALTER TABLE "catalog"."kubernetes_versions" ADD CONSTRAINT "kubernetes_versions_pk" PRIMARY KEY USING INDEX "kubernetes_versions_pk";

CREATE UNIQUE INDEX kubernetes_versions_uq_version ON catalog.kubernetes_versions USING btree (version);

ALTER TABLE "catalog"."kubernetes_versions" ADD CONSTRAINT "kubernetes_versions_uq_version" UNIQUE USING INDEX "kubernetes_versions_uq_version";

CREATE TABLE "catalog"."machine_types" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"name" text COLLATE "pg_catalog"."default" NOT NULL,
	"lcpu" integer NOT NULL,
	"memory" bigint NOT NULL
);

GRANT SELECT ON "catalog"."machine_types" TO "fun_authn_api";

GRANT SELECT ON "catalog"."machine_types" TO "fun_cluster_worker";

GRANT SELECT ON "catalog"."machine_types" TO "fun_fundament_api";

CREATE UNIQUE INDEX machine_types_pk ON catalog.machine_types USING btree (id);

ALTER TABLE "catalog"."machine_types" ADD CONSTRAINT "machine_types_pk" PRIMARY KEY USING INDEX "machine_types_pk";

CREATE UNIQUE INDEX machine_types_uq_name ON catalog.machine_types USING btree (name);

ALTER TABLE "catalog"."machine_types" ADD CONSTRAINT "machine_types_uq_name" UNIQUE USING INDEX "machine_types_uq_name";

CREATE TABLE "catalog"."region_kubernetes_versions" (
	"region_id" uuid NOT NULL,
	"kubernetes_version_id" uuid NOT NULL
);

GRANT SELECT ON "catalog"."region_kubernetes_versions" TO "fun_authn_api";

GRANT SELECT ON "catalog"."region_kubernetes_versions" TO "fun_cluster_worker";

GRANT SELECT ON "catalog"."region_kubernetes_versions" TO "fun_fundament_api";

ALTER TABLE "catalog"."region_kubernetes_versions" ADD CONSTRAINT "region_kubernetes_versions_fk_version" FOREIGN KEY (kubernetes_version_id) REFERENCES catalog.kubernetes_versions(id) ON UPDATE CASCADE ON DELETE RESTRICT NOT VALID;

ALTER TABLE "catalog"."region_kubernetes_versions" VALIDATE CONSTRAINT "region_kubernetes_versions_fk_version";

CREATE UNIQUE INDEX region_kubernetes_versions_pk ON catalog.region_kubernetes_versions USING btree (region_id, kubernetes_version_id);

ALTER TABLE "catalog"."region_kubernetes_versions" ADD CONSTRAINT "region_kubernetes_versions_pk" PRIMARY KEY USING INDEX "region_kubernetes_versions_pk";

CREATE TABLE "catalog"."region_machine_types" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"region_id" uuid NOT NULL,
	"machine_type_id" uuid NOT NULL
);

GRANT SELECT ON "catalog"."region_machine_types" TO "fun_authn_api";

GRANT SELECT ON "catalog"."region_machine_types" TO "fun_cluster_worker";

GRANT SELECT ON "catalog"."region_machine_types" TO "fun_fundament_api";

ALTER TABLE "catalog"."region_machine_types" ADD CONSTRAINT "region_machine_types_fk_machine_type" FOREIGN KEY (machine_type_id) REFERENCES catalog.machine_types(id) ON UPDATE CASCADE ON DELETE RESTRICT NOT VALID;

ALTER TABLE "catalog"."region_machine_types" VALIDATE CONSTRAINT "region_machine_types_fk_machine_type";

CREATE UNIQUE INDEX region_machine_types_pk ON catalog.region_machine_types USING btree (id);

ALTER TABLE "catalog"."region_machine_types" ADD CONSTRAINT "region_machine_types_pk" PRIMARY KEY USING INDEX "region_machine_types_pk";

CREATE UNIQUE INDEX region_machine_types_uq ON catalog.region_machine_types USING btree (region_id, machine_type_id);

ALTER TABLE "catalog"."region_machine_types" ADD CONSTRAINT "region_machine_types_uq" UNIQUE USING INDEX "region_machine_types_uq";

CREATE TABLE "catalog"."regions" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"name" text COLLATE "pg_catalog"."default" NOT NULL,
	"cloud_profile" text COLLATE "pg_catalog"."default" NOT NULL,
	"cloud_profile_region" text COLLATE "pg_catalog"."default" NOT NULL
);

GRANT SELECT ON "catalog"."regions" TO "fun_authn_api";

GRANT SELECT ON "catalog"."regions" TO "fun_cluster_worker";

GRANT SELECT ON "catalog"."regions" TO "fun_fundament_api";

CREATE UNIQUE INDEX regions_pk ON catalog.regions USING btree (id);

ALTER TABLE "catalog"."regions" ADD CONSTRAINT "regions_pk" PRIMARY KEY USING INDEX "regions_pk";

CREATE UNIQUE INDEX regions_uq_name ON catalog.regions USING btree (name);

ALTER TABLE "catalog"."regions" ADD CONSTRAINT "regions_uq_name" UNIQUE USING INDEX "regions_uq_name";

ALTER TABLE "catalog"."region_kubernetes_versions" ADD CONSTRAINT "region_kubernetes_versions_fk_region" FOREIGN KEY (region_id) REFERENCES catalog.regions(id) ON UPDATE CASCADE ON DELETE RESTRICT NOT VALID;

ALTER TABLE "catalog"."region_kubernetes_versions" VALIDATE CONSTRAINT "region_kubernetes_versions_fk_region";

ALTER TABLE "catalog"."region_machine_types" ADD CONSTRAINT "region_machine_types_fk_region" FOREIGN KEY (region_id) REFERENCES catalog.regions(id) ON UPDATE CASCADE ON DELETE RESTRICT NOT VALID;

ALTER TABLE "catalog"."region_machine_types" VALIDATE CONSTRAINT "region_machine_types_fk_region";

ALTER TABLE "tenant"."clusters" ADD COLUMN "kubernetes_version_id" uuid;

ALTER TABLE "tenant"."clusters" ADD COLUMN "region_id" uuid;

ALTER TABLE "tenant"."clusters" ADD CONSTRAINT "clusters_fk_region_version" FOREIGN KEY (region_id, kubernetes_version_id) REFERENCES catalog.region_kubernetes_versions(region_id, kubernetes_version_id) ON UPDATE CASCADE ON DELETE RESTRICT NOT VALID;

ALTER TABLE "tenant"."clusters" VALIDATE CONSTRAINT "clusters_fk_region_version";

ALTER TABLE "tenant"."node_pools" ADD COLUMN "region_machine_type_id" uuid;

ALTER TABLE "tenant"."node_pools" ADD CONSTRAINT "node_pools_fk_region_machine_type" FOREIGN KEY (region_machine_type_id) REFERENCES catalog.region_machine_types(id) ON UPDATE CASCADE ON DELETE RESTRICT NOT VALID;

ALTER TABLE "tenant"."node_pools" VALIDATE CONSTRAINT "node_pools_fk_region_machine_type";

CREATE OR REPLACE TRIGGER node_pool_outbox AFTER INSERT OR UPDATE OF name, machine_type, region_machine_type_id, autoscale_min, autoscale_max, deleted ON tenant.node_pools FOR EACH ROW EXECUTE FUNCTION tenant.node_pool_outbox_trigger();

CREATE CONSTRAINT TRIGGER region_match AFTER INSERT OR UPDATE ON tenant.node_pools NOT DEFERRABLE INITIALLY IMMEDIATE FOR EACH ROW EXECUTE FUNCTION tenant.node_pool_region_match_trigger();


-- Statements generated automatically, please review:
ALTER SCHEMA catalog OWNER TO fun_owner;
ALTER FUNCTION tenant.node_pool_region_match_trigger() OWNER TO fun_owner;
ALTER TABLE catalog.kubernetes_versions OWNER TO fun_owner;
ALTER TABLE catalog.machine_types OWNER TO fun_owner;
ALTER TABLE catalog.region_kubernetes_versions OWNER TO fun_owner;
ALTER TABLE catalog.region_machine_types OWNER TO fun_owner;
ALTER TABLE catalog.regions OWNER TO fun_owner;
