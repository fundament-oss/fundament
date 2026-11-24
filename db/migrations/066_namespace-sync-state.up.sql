SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

CREATE VIEW "tenant"."namespace_sync_state" AS
 SELECT DISTINCT ON (namespace_id) namespace_id,
    status AS outbox_status,
    retries AS outbox_retries,
    status_info AS outbox_error,
    id AS outbox_id
   FROM tenant.cluster_outbox
  WHERE namespace_id IS NOT NULL
  ORDER BY namespace_id, id DESC;;


-- Statements generated automatically, please review:
ALTER VIEW tenant.namespace_sync_state OWNER TO fun_fundament_api;
