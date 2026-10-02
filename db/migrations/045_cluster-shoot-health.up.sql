SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.cluster_outbox_update_cluster_status()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
DECLARE
    resolved_cluster_id uuid;
    resolved_error text;
BEGIN
    -- A pending row that carries status_info is deferred on a precondition
    -- (waiting, not failing); only retrying/failed rows surface as an error.
    IF NEW.status IN ('retrying', 'failed') THEN
        resolved_error := NEW.status_info;
    END IF;

    IF NEW.cluster_id IS NOT NULL THEN
        UPDATE tenant.clusters
        SET outbox_status = NEW.status,
            outbox_retries = NEW.retries,
            outbox_error = resolved_error
        WHERE tenant.clusters.id = NEW.cluster_id;
    ELSIF NEW.node_pool_id IS NOT NULL THEN
        SELECT tenant.node_pools.cluster_id INTO resolved_cluster_id
        FROM tenant.node_pools
        WHERE tenant.node_pools.id = NEW.node_pool_id;

        IF resolved_cluster_id IS NOT NULL THEN
            UPDATE tenant.clusters
            SET outbox_status = NEW.status,
                outbox_retries = NEW.retries,
                outbox_error = resolved_error
            WHERE tenant.clusters.id = resolved_cluster_id;
        END IF;
    END IF;
    RETURN NULL;
END;
$function$
;

ALTER TABLE "tenant"."cluster_events" DROP CONSTRAINT "cluster_events_ck_event_type";

ALTER TABLE "tenant"."cluster_events" ADD CONSTRAINT "cluster_events_ck_event_type" CHECK((event_type = ANY (ARRAY['sync_requested'::text, 'sync_claimed'::text, 'sync_succeeded'::text, 'sync_failed'::text, 'status_progressing'::text, 'status_ready'::text, 'status_error'::text, 'status_deleted'::text, 'status_healthy'::text, 'status_unhealthy'::text, 'status_warning'::text, 'user_sync_succeeded'::text, 'user_sync_failed'::text]))) NOT VALID;

ALTER TABLE "tenant"."cluster_events" VALIDATE CONSTRAINT "cluster_events_ck_event_type";

ALTER TABLE "tenant"."clusters" ADD COLUMN "shoot_health" text COLLATE "pg_catalog"."default";

ALTER TABLE "tenant"."clusters" ADD CONSTRAINT "clusters_ck_shoot_health" CHECK((shoot_health = ANY (ARRAY['healthy'::text, 'unhealthy'::text]))) NOT VALID;

ALTER TABLE "tenant"."clusters" VALIDATE CONSTRAINT "clusters_ck_shoot_health";

