SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

-- Add denormalized outbox state columns to clusters table.
-- These are maintained by a trigger on cluster_outbox.
ALTER TABLE "tenant"."clusters" ADD COLUMN "outbox_status" text;
ALTER TABLE "tenant"."clusters" ADD COLUMN "outbox_retries" integer NOT NULL DEFAULT 0;
ALTER TABLE "tenant"."clusters" ADD COLUMN "outbox_error" text;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.cluster_outbox_update_cluster_status()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    IF NEW.cluster_id IS NOT NULL THEN
        UPDATE tenant.clusters
        SET outbox_status = latest.status,
            outbox_retries = latest.retries,
            outbox_error = latest.status_info
        FROM (
            SELECT status, retries, status_info
            FROM tenant.cluster_outbox
            WHERE cluster_id = NEW.cluster_id
            ORDER BY id DESC
            LIMIT 1
        ) latest
        WHERE tenant.clusters.id = NEW.cluster_id;
    END IF;
    RETURN NULL;
END;
$function$
;

ALTER FUNCTION tenant.cluster_outbox_update_cluster_status() OWNER TO fun_owner;

CREATE TRIGGER cluster_outbox_update_cluster_status AFTER INSERT OR UPDATE ON tenant.cluster_outbox FOR EACH ROW EXECUTE FUNCTION tenant.cluster_outbox_update_cluster_status();

-- Backfill existing clusters from current outbox state.
UPDATE tenant.clusters
SET outbox_status = sub.status,
    outbox_retries = sub.retries,
    outbox_error = sub.status_info
FROM (
    SELECT DISTINCT ON (cluster_id) cluster_id, status, retries, status_info
    FROM tenant.cluster_outbox
    ORDER BY cluster_id, id DESC
) sub
WHERE tenant.clusters.id = sub.cluster_id;
