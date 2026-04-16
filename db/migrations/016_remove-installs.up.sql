SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

ALTER TABLE "appstore"."installs" DROP CONSTRAINT "installs_fk_cluster";

ALTER TABLE "appstore"."installs" DROP CONSTRAINT "installs_fk_plugin";

ALTER TABLE "authz"."outbox" DROP CONSTRAINT "outbox_fk_install";

ALTER TABLE "authz"."outbox" DROP CONSTRAINT "outbox_ck_single_fk";

/* Hazards:
 - DELETES_DATA: Deletes all values in the column
*/
ALTER TABLE "authz"."outbox" DROP COLUMN "install_id";

ALTER TABLE "authz"."outbox" ADD CONSTRAINT "outbox_ck_single_fk" CHECK((num_nonnulls(project_id, project_member_id, cluster_id, node_pool_id, namespace_id, api_key_id, organization_user_id) = 1)) NOT VALID;

ALTER TABLE "authz"."outbox" VALIDATE CONSTRAINT "outbox_ck_single_fk";

ALTER TABLE "tenant"."idempotency_keys" DROP CONSTRAINT "idempotency_keys_fk_install";

ALTER TABLE "tenant"."idempotency_keys" DROP CONSTRAINT "idempotency_keys_ck_single_fk";

/* Hazards:
 - DELETES_DATA: Deletes all values in the column
*/
ALTER TABLE "tenant"."idempotency_keys" DROP COLUMN "install_id";

ALTER TABLE "tenant"."idempotency_keys" ADD CONSTRAINT "idempotency_keys_ck_single_fk" CHECK((num_nonnulls(project_id, project_member_id, cluster_id, node_pool_id, namespace_id, api_key_id, organization_user_id) <= 1)) NOT VALID;

ALTER TABLE "tenant"."idempotency_keys" VALIDATE CONSTRAINT "idempotency_keys_ck_single_fk";

DROP TRIGGER "installs_outbox" ON "appstore"."installs";

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For drops, this means you need to ensure that all functions this function depends on are dropped after this statement.
*/
DROP FUNCTION "authz"."installs_sync_trigger"();

SET SESSION statement_timeout = 1200000;

/* Hazards:
 - DELETES_DATA: Deletes all rows in the table (and the table itself)
*/
DROP TABLE "appstore"."installs";

