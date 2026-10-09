SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.organization_limits_outbox_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 SECURITY DEFINER COST 1
AS $function$
BEGIN
    -- Reconcile each active namespace's LimitRange. A deleted change only
    -- matters when the row carries default values in OLD or NEW; otherwise the
    -- reconcile would be a guaranteed no-op.
    IF (TG_OP = 'INSERT' AND (NEW.default_memory_request_mi IS NOT NULL
                              OR NEW.default_memory_limit_mi IS NOT NULL
                              OR NEW.default_cpu_request_m IS NOT NULL
                              OR NEW.default_cpu_limit_m IS NOT NULL))
       OR (TG_OP = 'UPDATE' AND (OLD.default_memory_request_mi IS DISTINCT FROM NEW.default_memory_request_mi
                                 OR OLD.default_memory_limit_mi IS DISTINCT FROM NEW.default_memory_limit_mi
                                 OR OLD.default_cpu_request_m IS DISTINCT FROM NEW.default_cpu_request_m
                                 OR OLD.default_cpu_limit_m IS DISTINCT FROM NEW.default_cpu_limit_m
                                 OR (OLD.deleted IS DISTINCT FROM NEW.deleted
                                     AND (OLD.default_memory_request_mi IS NOT NULL
                                          OR OLD.default_memory_limit_mi IS NOT NULL
                                          OR OLD.default_cpu_request_m IS NOT NULL
                                          OR OLD.default_cpu_limit_m IS NOT NULL
                                          OR NEW.default_memory_request_mi IS NOT NULL
                                          OR NEW.default_memory_limit_mi IS NOT NULL
                                          OR NEW.default_cpu_request_m IS NOT NULL
                                          OR NEW.default_cpu_limit_m IS NOT NULL))))
    THEN
        INSERT INTO tenant.cluster_outbox (namespace_id, event, source)
        SELECT tenant.namespaces.id, 'updated', 'trigger'
        FROM tenant.namespaces
        JOIN tenant.projects ON tenant.projects.id = tenant.namespaces.project_id
        JOIN tenant.clusters ON tenant.clusters.id = tenant.projects.cluster_id
        WHERE tenant.clusters.organization_id = NEW.organization_id
          AND tenant.namespaces.deleted IS NULL;
    END IF;

    RETURN NULL;
END;
$function$
;

ALTER TABLE "tenant"."organization_limits" DROP CONSTRAINT "organization_limits_ck_max_node_pools_per_cluster";

ALTER TABLE "tenant"."organization_limits" DROP CONSTRAINT "organization_limits_ck_max_nodes_per_cluster";

ALTER TABLE "tenant"."organization_limits" DROP CONSTRAINT "organization_limits_ck_max_nodes_per_node_pool";

/* Hazards:
 - DELETES_DATA: Deletes all values in the column
*/
ALTER TABLE "tenant"."organization_limits" DROP COLUMN "max_node_pools_per_cluster";

/* Hazards:
 - DELETES_DATA: Deletes all values in the column
*/
ALTER TABLE "tenant"."organization_limits" DROP COLUMN "max_nodes_per_cluster";

/* Hazards:
 - DELETES_DATA: Deletes all values in the column
*/
ALTER TABLE "tenant"."organization_limits" DROP COLUMN "max_nodes_per_node_pool";

