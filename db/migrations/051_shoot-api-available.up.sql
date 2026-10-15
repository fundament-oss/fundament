SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

ALTER TABLE "tenant"."cluster_events" DROP CONSTRAINT "cluster_events_ck_event_type";

ALTER TABLE "tenant"."cluster_events" ADD CONSTRAINT "cluster_events_ck_event_type" CHECK((event_type = ANY (ARRAY['sync_requested'::text, 'sync_claimed'::text, 'sync_succeeded'::text, 'sync_failed'::text, 'status_progressing'::text, 'status_ready'::text, 'status_error'::text, 'status_deleted'::text, 'status_healthy'::text, 'status_unhealthy'::text, 'status_warning'::text, 'status_lost'::text, 'status_api_available'::text, 'user_sync_succeeded'::text, 'user_sync_failed'::text]))) NOT VALID;

ALTER TABLE "tenant"."cluster_events" VALIDATE CONSTRAINT "cluster_events_ck_event_type";

ALTER TABLE "tenant"."cluster_outbox" DROP CONSTRAINT "cluster_outbox_ck_event";

ALTER TABLE "tenant"."cluster_outbox" ADD CONSTRAINT "cluster_outbox_ck_event" CHECK((event = ANY (ARRAY['created'::text, 'updated'::text, 'deleted'::text, 'reconcile'::text, 'ready'::text, 'api_available'::text]))) NOT VALID;

ALTER TABLE "tenant"."cluster_outbox" VALIDATE CONSTRAINT "cluster_outbox_ck_event";

ALTER TABLE "tenant"."clusters" ADD COLUMN "shoot_api_available" timestamp with time zone;

