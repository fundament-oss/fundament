SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - DELETES_DATA: Deletes all values in the column
*/
ALTER TABLE "tenant"."clusters" DROP COLUMN "outbox_error";

/* Hazards:
 - DELETES_DATA: Deletes all values in the column
*/
ALTER TABLE "tenant"."clusters" DROP COLUMN "outbox_retries";

/* Hazards:
 - DELETES_DATA: Deletes all values in the column
*/
ALTER TABLE "tenant"."clusters" DROP COLUMN "outbox_status";

DROP TRIGGER "cluster_outbox_update_cluster_status" ON "tenant"."cluster_outbox";

CREATE VIEW "tenant"."cluster_sync_state" AS
 SELECT DISTINCT ON (cluster_id) cluster_id,
    event AS outbox_event,
    status AS outbox_status,
    retries AS outbox_retries,
    status_info AS outbox_error,
    id AS outbox_id
   FROM tenant.cluster_outbox
  WHERE cluster_id IS NOT NULL AND (event = ANY (ARRAY['created'::text, 'updated'::text, 'deleted'::text, 'reconcile'::text]))
  ORDER BY cluster_id, id DESC;;

CREATE VIEW "tenant"."node_pool_sync_state" AS
 SELECT DISTINCT ON (node_pool_id) node_pool_id,
    status AS outbox_status,
    retries AS outbox_retries,
    status_info AS outbox_error,
    id AS outbox_id
   FROM tenant.cluster_outbox
  WHERE node_pool_id IS NOT NULL
  ORDER BY node_pool_id, id DESC;;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For drops, this means you need to ensure that all functions this function depends on are dropped after this statement.
*/
DROP FUNCTION "tenant"."cluster_outbox_update_cluster_status"();


-- Statements generated automatically, please review:
ALTER VIEW tenant.cluster_sync_state OWNER TO fun_fundament_api;
ALTER VIEW tenant.node_pool_sync_state OWNER TO fun_fundament_api;
