SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

ALTER INDEX "tenant"."users_uq_external_id" RENAME TO "pgschemadiff_tmpidx_users_uq_external_id_bpGqLi6qTPqFhOo7IGQNfw";

ALTER TABLE "tenant"."users" ADD COLUMN "deleted" timestamp with time zone;

ALTER TABLE "tenant"."users" ADD COLUMN "email" text COLLATE "pg_catalog"."default";

ALTER TABLE "tenant"."users" ALTER COLUMN "external_id" DROP NOT NULL;

ALTER TABLE "tenant"."users" ADD COLUMN "role" text COLLATE "pg_catalog"."default" DEFAULT 'viewer'::text NOT NULL;

/* Hazards:
 - ACQUIRES_SHARE_LOCK: Non-concurrent index creates will lock out writes to the table during the duration of the index build.
*/
CREATE UNIQUE INDEX users_uq_external_id ON tenant.users USING btree (external_id, deleted) NULLS NOT DISTINCT;

ALTER TABLE "tenant"."users" ADD CONSTRAINT "users_uq_external_id" UNIQUE USING INDEX "users_uq_external_id";

/* Hazards:
 - ACQUIRES_ACCESS_EXCLUSIVE_LOCK: Index drops will lock out all accesses to the table. They should be fast.
 - INDEX_DROPPED: Dropping this index means queries that use this index might perform worse because they will no longer will be able to leverage it.
*/
ALTER TABLE "tenant"."users" DROP CONSTRAINT "pgschemadiff_tmpidx_users_uq_external_id_bpGqLi6qTPqFhOo7IGQNfw";
