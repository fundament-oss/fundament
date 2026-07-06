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
    -- Node-cap branch: re-apply each active cluster's shoot spec. A deleted
    -- change only matters when the row carries node-cap values in OLD or NEW;
    -- otherwise the re-apply would be a guaranteed no-op.
    IF (TG_OP = 'INSERT' AND (NEW.max_nodes_per_cluster IS NOT NULL
                              OR NEW.max_node_pools_per_cluster IS NOT NULL
                              OR NEW.max_nodes_per_node_pool IS NOT NULL))
       OR (TG_OP = 'UPDATE' AND (OLD.max_nodes_per_cluster IS DISTINCT FROM NEW.max_nodes_per_cluster
                                 OR OLD.max_node_pools_per_cluster IS DISTINCT FROM NEW.max_node_pools_per_cluster
                                 OR OLD.max_nodes_per_node_pool IS DISTINCT FROM NEW.max_nodes_per_node_pool
                                 OR (OLD.deleted IS DISTINCT FROM NEW.deleted
                                     AND (OLD.max_nodes_per_cluster IS NOT NULL
                                          OR OLD.max_node_pools_per_cluster IS NOT NULL
                                          OR OLD.max_nodes_per_node_pool IS NOT NULL
                                          OR NEW.max_nodes_per_cluster IS NOT NULL
                                          OR NEW.max_node_pools_per_cluster IS NOT NULL
                                          OR NEW.max_nodes_per_node_pool IS NOT NULL))))
    THEN
        INSERT INTO tenant.cluster_outbox (cluster_id, event, source)
        SELECT tenant.clusters.id, 'updated', 'trigger'
        FROM tenant.clusters
        WHERE tenant.clusters.organization_id = NEW.organization_id
          AND tenant.clusters.deleted IS NULL;
    END IF;

    -- Per-container-default branch: reconcile each active namespace's
    -- LimitRange. Same deleted-change scoping as above, on the default columns.
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

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.project_limits_outbox_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 SECURITY DEFINER COST 1
AS $function$
BEGIN
    -- A deleted change only matters when the row carries default values in
    -- OLD or NEW; otherwise the LimitRange reconcile would be a no-op.
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
        WHERE tenant.namespaces.project_id = NEW.project_id
          AND tenant.namespaces.deleted IS NULL;
    END IF;

    RETURN NULL;
END;
$function$
;

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "organization_limits_cluster_worker_read" ON "tenant"."organization_limits"
	AS PERMISSIVE
	FOR SELECT
	TO fun_cluster_worker
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."organization_limits" TO "fun_cluster_worker";

CREATE TRIGGER organization_limits_outbox AFTER INSERT OR UPDATE ON tenant.organization_limits FOR EACH ROW EXECUTE FUNCTION tenant.organization_limits_outbox_trigger();

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "project_limits_cluster_worker_read" ON "tenant"."project_limits"
	AS PERMISSIVE
	FOR SELECT
	TO fun_cluster_worker
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."project_limits" TO "fun_cluster_worker";

CREATE TRIGGER project_limits_outbox AFTER INSERT OR UPDATE ON tenant.project_limits FOR EACH ROW EXECUTE FUNCTION tenant.project_limits_outbox_trigger();


-- Statements generated automatically, please review:
ALTER FUNCTION tenant.organization_limits_outbox_trigger() OWNER TO fun_owner;
ALTER FUNCTION tenant.project_limits_outbox_trigger() OWNER TO fun_owner;
