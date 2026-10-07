SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

ALTER TABLE "tenant"."cluster_events" DROP CONSTRAINT "cluster_events_ck_event_type";

ALTER TABLE "tenant"."cluster_events" ADD CONSTRAINT "cluster_events_ck_event_type" CHECK((event_type = ANY (ARRAY['sync_requested'::text, 'sync_claimed'::text, 'sync_succeeded'::text, 'sync_failed'::text, 'status_progressing'::text, 'status_ready'::text, 'status_error'::text, 'status_deleted'::text, 'status_healthy'::text, 'status_unhealthy'::text, 'status_warning'::text, 'user_sync_succeeded'::text, 'user_sync_failed'::text, 'nodepool_waiting'::text, 'nodepool_error'::text, 'nodepool_ready'::text]))) NOT VALID;

ALTER TABLE "tenant"."cluster_events" VALIDATE CONSTRAINT "cluster_events_ck_event_type";

ALTER TABLE "tenant"."node_pools" ADD COLUMN "status" text COLLATE "pg_catalog"."default";

ALTER TABLE "tenant"."node_pools" ADD CONSTRAINT "node_pools_ck_status" CHECK((status = ANY (ARRAY['progressing'::text, 'waiting'::text, 'error'::text, 'ready'::text]))) NOT VALID;

ALTER TABLE "tenant"."node_pools" VALIDATE CONSTRAINT "node_pools_ck_status";

ALTER TABLE "tenant"."node_pools" ADD COLUMN "status_message" text COLLATE "pg_catalog"."default";

ALTER TABLE "tenant"."node_pools" ADD COLUMN "status_updated" timestamp with time zone;

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "node_pools_cluster_worker_update" ON "tenant"."node_pools"
	AS PERMISSIVE
	FOR UPDATE
	TO fun_cluster_worker
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "tenant"."node_pools" TO "fun_cluster_worker";

