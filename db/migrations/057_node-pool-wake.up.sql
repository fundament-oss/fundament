SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.cluster_outbox_cluster_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 SECURITY DEFINER COST 1
AS $function$
BEGIN
    IF TG_OP = 'INSERT'
       OR OLD.deleted IS DISTINCT FROM NEW.deleted
       OR OLD.region IS DISTINCT FROM NEW.region
       OR OLD.kubernetes_version IS DISTINCT FROM NEW.kubernetes_version
    THEN
        INSERT INTO tenant.cluster_outbox (cluster_id, event, source)
        VALUES (COALESCE(NEW.id, OLD.id),
                CASE
                    WHEN TG_OP = 'INSERT' THEN 'created'
                    WHEN OLD.deleted IS NULL AND NEW.deleted IS NOT NULL THEN 'deleted'
                    ELSE 'updated'
                END,
                'trigger');
    END IF;
    -- A row parked until the cluster's first node pool would wait forever once
    -- the cluster is deleted: wake the cluster's parked rows, so they run the
    -- delete path and complete. This update holds the cluster row, which a
    -- worker about to park locks before it rechecks (see the node pool trigger).
    IF TG_OP = 'UPDATE' AND OLD.deleted IS NULL AND NEW.deleted IS NOT NULL THEN
        UPDATE tenant.cluster_outbox
        SET retry_after = now()
        WHERE tenant.cluster_outbox.cluster_id = NEW.id
          AND tenant.cluster_outbox.status IN ('pending', 'retrying')
          AND tenant.cluster_outbox.retry_after = 'infinity';
    END IF;
    RETURN NEW;
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.node_pool_outbox_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 SECURITY DEFINER COST 1
AS $function$
BEGIN
    INSERT INTO tenant.cluster_outbox (node_pool_id, event, source)
    VALUES (
        COALESCE(NEW.id, OLD.id),
        CASE
            WHEN TG_OP = 'INSERT' THEN 'created'
            WHEN OLD.deleted IS NULL AND NEW.deleted IS NOT NULL THEN 'deleted'
            ELSE 'updated'
        END,
        'trigger'
    );
    -- Wake the cluster's rows parked until its first pool. The cluster row is
    -- locked first: a worker about to park locks it too and rechecks the pools
    -- under that lock, so a pool is either seen by the recheck or finds the
    -- row parked. Only parked rows are touched: a claimed row is never one, so
    -- this never waits for a sync in progress.
    IF TG_OP = 'INSERT' AND NEW.deleted IS NULL THEN
        PERFORM 1 FROM tenant.clusters
        WHERE tenant.clusters.id = NEW.cluster_id
        FOR NO KEY UPDATE;
        UPDATE tenant.cluster_outbox
        SET retry_after = now()
        WHERE tenant.cluster_outbox.cluster_id = NEW.cluster_id
          AND tenant.cluster_outbox.status IN ('pending', 'retrying')
          AND tenant.cluster_outbox.retry_after = 'infinity';
    END IF;
    RETURN NULL;
END;
$function$
;

ALTER TABLE "tenant"."cluster_events" DROP CONSTRAINT "cluster_events_ck_event_type";

ALTER TABLE "tenant"."cluster_events" ADD CONSTRAINT "cluster_events_ck_event_type" CHECK((event_type = ANY (ARRAY['sync_requested'::text, 'sync_claimed'::text, 'sync_succeeded'::text, 'sync_failed'::text, 'status_progressing'::text, 'status_ready'::text, 'status_error'::text, 'status_deleted'::text, 'status_healthy'::text, 'status_unhealthy'::text, 'status_warning'::text, 'status_lost'::text, 'status_api_available'::text, 'user_sync_succeeded'::text, 'user_sync_failed'::text, 'nodepool_waiting'::text, 'nodepool_error'::text, 'nodepool_ready'::text, 'sync_waiting_for_precondition'::text]))) NOT VALID;

ALTER TABLE "tenant"."cluster_events" VALIDATE CONSTRAINT "cluster_events_ck_event_type";

