SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

ALTER TABLE "tenant"."node_pools" ADD COLUMN "node_count" integer;

ALTER TABLE "tenant"."node_pools" ADD COLUMN "node_count_ready" integer;

ALTER TABLE "tenant"."node_pools" ADD COLUMN "node_count_updated" timestamp with time zone;

ALTER TABLE "tenant"."node_pools" ADD CONSTRAINT "node_pools_ck_node_count" CHECK((((node_count IS NULL) AND (node_count_ready IS NULL) AND (node_count_updated IS NULL)) OR ((node_count IS NOT NULL) AND (node_count_ready IS NOT NULL) AND (node_count_updated IS NOT NULL) AND (node_count_ready >= 0) AND (node_count_ready <= node_count)))) NOT VALID;

ALTER TABLE "tenant"."node_pools" VALIDATE CONSTRAINT "node_pools_ck_node_count";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ("node_count") ON "tenant"."node_pools" TO "fun_cluster_worker";

GRANT UPDATE ("node_count_ready") ON "tenant"."node_pools" TO "fun_cluster_worker";

GRANT UPDATE ("node_count_updated") ON "tenant"."node_pools" TO "fun_cluster_worker";
