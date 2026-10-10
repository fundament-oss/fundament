SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.clusters_tr_verify_quota()
 RETURNS trigger
 LANGUAGE plpgsql
 SECURITY DEFINER COST 1
AS $function$
DECLARE
    v_quota integer;
BEGIN
    -- Only creation is checked: a quota lowered below what an organization
    -- already runs leaves its clusters alone and stops the next one.
    IF NEW.deleted IS NOT NULL THEN
        RETURN NULL;
    END IF;

    -- FOR NO KEY UPDATE serialises the count with other cluster inserts for
    -- this organization, without blocking the key-share locks of foreign keys.
    SELECT tenant.organizations.quota_clusters INTO v_quota
    FROM tenant.organizations
    WHERE tenant.organizations.id = NEW.organization_id
    FOR NO KEY UPDATE;

    IF tenant.organization_clusters_in_use(NEW.organization_id) > v_quota THEN
        RAISE EXCEPTION 'the organization has reached its quota of % cluster(s)', v_quota
                    USING HINT = 'cluster_quota_exceeded';
    END IF;
    RETURN NULL;
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.node_pools_tr_verify_quota()
 RETURNS trigger
 LANGUAGE plpgsql
 SECURITY DEFINER COST 1
AS $function$
DECLARE
    v_organization_id uuid;
    v_quota integer;
    v_in_use bigint;
    v_machine_type text;
    v_region text;
BEGIN
    -- A pool without a catalog reference cannot be matched to a quota entry;
    -- see organization_nodes_in_use.
    IF NEW.deleted IS NOT NULL OR NEW.region_machine_type_id IS NULL THEN
        RETURN NULL;
    END IF;

    -- Only growth is checked: a pool that shrinks or keeps its maximum stays
    -- valid under a quota lowered below what the organization already runs.
    IF TG_OP = 'UPDATE'
       AND NEW.autoscale_max <= OLD.autoscale_max
       AND NEW.region_machine_type_id IS NOT DISTINCT FROM OLD.region_machine_type_id
    THEN
        RETURN NULL;
    END IF;

    SELECT tenant.clusters.organization_id INTO v_organization_id
    FROM tenant.clusters
    WHERE tenant.clusters.id = NEW.cluster_id;

    -- Serialises the sum with the other node pool changes of the organization.
    PERFORM 1
    FROM tenant.organizations
    WHERE tenant.organizations.id = v_organization_id
    FOR NO KEY UPDATE;

    SELECT tenant.organization_machine_quotas.max_nodes INTO v_quota
    FROM tenant.organization_machine_quotas
    WHERE tenant.organization_machine_quotas.organization_id = v_organization_id
      AND tenant.organization_machine_quotas.region_machine_type_id = NEW.region_machine_type_id;
    v_quota := coalesce(v_quota, 0);

    v_in_use := tenant.organization_nodes_in_use(v_organization_id, NEW.region_machine_type_id);
    IF v_in_use > v_quota THEN
        SELECT catalog.machine_types.name, catalog.regions.name INTO v_machine_type, v_region
        FROM catalog.region_machine_types
        JOIN catalog.machine_types ON catalog.machine_types.id = catalog.region_machine_types.machine_type_id
        JOIN catalog.regions ON catalog.regions.id = catalog.region_machine_types.region_id
        WHERE catalog.region_machine_types.id = NEW.region_machine_type_id;

        IF v_quota = 0 THEN
            RAISE EXCEPTION 'the organization has no quota for machine type % in region %', v_machine_type, v_region
                        USING HINT = 'node_pool_quota_exceeded';
        END IF;
        RAISE EXCEPTION 'node pools of machine type % in region % would total % nodes, over the organization''s quota of %', v_machine_type, v_region, v_in_use, v_quota
                    USING HINT = 'node_pool_quota_exceeded';
    END IF;
    RETURN NULL;
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.organization_clusters_in_use(p_organization_id uuid)
 RETURNS bigint
 LANGUAGE plpgsql
 STABLE SECURITY DEFINER COST 1
AS $function$
BEGIN
    -- What counts against quota_clusters: the organization's live clusters, and
    -- its deleted ones until Gardener confirms the shoot is gone, since those
    -- still hold machines. The same definition as OrganizationCountLiveClusters
    -- in funops.
    RETURN (
        SELECT count(*)
        FROM tenant.clusters
        WHERE tenant.clusters.organization_id = p_organization_id
          AND (tenant.clusters.deleted IS NULL OR tenant.clusters.shoot_status IS DISTINCT FROM 'deleted')
    );
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.organization_nodes_in_use(p_organization_id uuid, p_region_machine_type_id uuid)
 RETURNS bigint
 LANGUAGE plpgsql
 STABLE SECURITY DEFINER COST 1
AS $function$
BEGIN
    -- What counts against a machine quota: the autoscale maxima of the live
    -- node pools of that machine type and region on the organization's
    -- clusters, deleted clusters included until their shoot is gone. A pool
    -- without a catalog reference (pre-catalog rows) has nothing to count
    -- against and is skipped, as node_pool_region_match_trigger skips it.
    RETURN (
        SELECT coalesce(sum(tenant.node_pools.autoscale_max), 0)
        FROM tenant.node_pools
        JOIN tenant.clusters ON tenant.clusters.id = tenant.node_pools.cluster_id
        WHERE tenant.clusters.organization_id = p_organization_id
          AND tenant.node_pools.region_machine_type_id = p_region_machine_type_id
          AND tenant.node_pools.deleted IS NULL
          AND (tenant.clusters.deleted IS NULL OR tenant.clusters.shoot_status IS DISTINCT FROM 'deleted')
    );
END;
$function$
;

CREATE CONSTRAINT TRIGGER verify_quota AFTER INSERT ON tenant.clusters NOT DEFERRABLE INITIALLY IMMEDIATE FOR EACH ROW EXECUTE FUNCTION tenant.clusters_tr_verify_quota();

CREATE CONSTRAINT TRIGGER verify_quota AFTER INSERT OR UPDATE OF autoscale_max, region_machine_type_id ON tenant.node_pools NOT DEFERRABLE INITIALLY IMMEDIATE FOR EACH ROW EXECUTE FUNCTION tenant.node_pools_tr_verify_quota();

CREATE TABLE "tenant"."organization_machine_quotas" (
	"organization_id" uuid NOT NULL,
	"region_machine_type_id" uuid NOT NULL,
	"max_nodes" integer NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"updated" timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE "tenant"."organization_machine_quotas" ADD CONSTRAINT "organization_machine_quotas_ck_max_nodes" CHECK((max_nodes >= 0));

ALTER TABLE "tenant"."organization_machine_quotas" ENABLE ROW LEVEL SECURITY;

ALTER TABLE "tenant"."organization_machine_quotas" ADD CONSTRAINT "organization_machine_quotas_fk_region_machine_type" FOREIGN KEY (region_machine_type_id) REFERENCES catalog.region_machine_types(id) ON UPDATE CASCADE ON DELETE RESTRICT NOT VALID;

ALTER TABLE "tenant"."organization_machine_quotas" VALIDATE CONSTRAINT "organization_machine_quotas_fk_region_machine_type";

CREATE UNIQUE INDEX organization_machine_quotas_pk ON tenant.organization_machine_quotas USING btree (organization_id, region_machine_type_id);

ALTER TABLE "tenant"."organization_machine_quotas" ADD CONSTRAINT "organization_machine_quotas_pk" PRIMARY KEY USING INDEX "organization_machine_quotas_pk";

ALTER TABLE "tenant"."organizations" ADD COLUMN "quota_clusters" integer DEFAULT 1 NOT NULL;

ALTER TABLE "tenant"."organizations" ADD CONSTRAINT "organizations_ck_quota_clusters" CHECK((quota_clusters >= 0)) NOT VALID;

ALTER TABLE "tenant"."organizations" VALIDATE CONSTRAINT "organizations_ck_quota_clusters";

ALTER TABLE "tenant"."organization_machine_quotas" ADD CONSTRAINT "organization_machine_quotas_fk_organization" FOREIGN KEY (organization_id) REFERENCES tenant.organizations(id) NOT VALID;

ALTER TABLE "tenant"."organization_machine_quotas" VALIDATE CONSTRAINT "organization_machine_quotas_fk_organization";


-- Statements generated automatically, please review:
ALTER FUNCTION tenant.clusters_tr_verify_quota() OWNER TO fun_owner;
ALTER FUNCTION tenant.node_pools_tr_verify_quota() OWNER TO fun_owner;
ALTER FUNCTION tenant.organization_clusters_in_use(p_organization_id uuid) OWNER TO fun_owner;
ALTER FUNCTION tenant.organization_nodes_in_use(p_organization_id uuid, p_region_machine_type_id uuid) OWNER TO fun_owner;
ALTER TABLE tenant.organization_machine_quotas OWNER TO fun_owner;
