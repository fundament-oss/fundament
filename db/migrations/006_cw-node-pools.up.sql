SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.node_pool_reset_cluster_synced()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    UPDATE tenant.clusters
    SET synced = NULL,
        sync_claimed_at = NULL,
        sync_attempts = 0,
        sync_error = NULL
    WHERE id = COALESCE(NEW.cluster_id, OLD.cluster_id);
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "node_pools_cluster_worker_read" ON "tenant"."node_pools"
	AS PERMISSIVE
	FOR SELECT
	TO fun_cluster_worker
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."node_pools" TO "fun_cluster_worker";

-- Fires on INSERT and relevant UPDATE columns, including `deleted` for soft-deletes
-- (there is no hard DELETE on node_pools; removal is done by setting deleted IS NOT NULL).
CREATE TRIGGER node_pool_reset_cluster_synced AFTER INSERT OR UPDATE OF name, machine_type, autoscale_min, autoscale_max, deleted ON tenant.node_pools FOR EACH ROW EXECUTE FUNCTION tenant.node_pool_reset_cluster_synced();


-- Statements generated automatically, please review:
ALTER FUNCTION tenant.node_pool_reset_cluster_synced() OWNER TO fun_owner;
