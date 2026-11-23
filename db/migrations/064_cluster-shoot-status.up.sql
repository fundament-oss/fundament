SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

ALTER TABLE "tenant"."clusters" ADD CONSTRAINT "clusters_ck_shoot_status" CHECK((shoot_status = ANY (ARRAY['pending'::text, 'progressing'::text, 'ready'::text, 'error'::text, 'deleting'::text, 'deleted'::text, 'lost'::text]))) NOT VALID;

ALTER TABLE "tenant"."clusters" VALIDATE CONSTRAINT "clusters_ck_shoot_status";

ALTER TABLE "tenant"."clusters" ADD COLUMN "shoot_status_since" timestamp with time zone;

/* Hazards:
 - ACQUIRES_ACCESS_EXCLUSIVE_LOCK: Index drops will lock out all accesses to the table. They should be fast.
 - INDEX_DROPPED: Dropping this index means queries that use this index might perform worse because they will no longer will be able to leverage it.
*/
DROP INDEX "tenant"."clusters_idx_needs_sync";

