SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.cluster_outbox_cluster_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
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
    RETURN NEW;
END;
$function$
;

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

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.node_pool_outbox_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    INSERT INTO tenant.cluster_outbox (cluster_id, event, source)
    VALUES (
        COALESCE(NEW.cluster_id, OLD.cluster_id),
        CASE
            WHEN TG_OP = 'INSERT' THEN 'created'
            WHEN OLD.deleted IS NULL AND NEW.deleted IS NOT NULL THEN 'deleted'
            ELSE 'updated'
        END,
        'node_pool'
    );
    RETURN NULL;
END;
$function$
;

ALTER INDEX "tenant"."cluster_outbox_idx_cluster_id" RENAME TO "pgschemadiff_tmpidx_cluster_outbox_idx_c_UxODa3TTT_Sk3eQpqhLY1g";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."cluster_outbox" TO "fun_fundament_api";

ALTER TABLE "tenant"."cluster_outbox" DROP CONSTRAINT "cluster_outbox_ck_source";

ALTER TABLE "tenant"."cluster_outbox" ADD CONSTRAINT "cluster_outbox_ck_source" CHECK((source = ANY (ARRAY['trigger'::text, 'reconcile'::text, 'manual'::text, 'node_pool'::text]))) NOT VALID;

ALTER TABLE "tenant"."cluster_outbox" VALIDATE CONSTRAINT "cluster_outbox_ck_source";

/* Hazards:
 - ACQUIRES_SHARE_LOCK: Non-concurrent index creates will lock out writes to the table during the duration of the index build.
*/
CREATE INDEX cluster_outbox_idx_cluster_id ON tenant.cluster_outbox USING btree (cluster_id, id DESC NULLS LAST);

CREATE TRIGGER cluster_outbox_update_cluster_status AFTER INSERT OR UPDATE ON tenant.cluster_outbox FOR EACH ROW EXECUTE FUNCTION tenant.cluster_outbox_update_cluster_status();

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "organizations_cluster_worker_policy" ON "tenant"."organizations"
	AS PERMISSIVE
	FOR SELECT
	TO fun_cluster_worker
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "projects_cluster_worker_policy" ON "tenant"."projects"
	AS PERMISSIVE
	FOR SELECT
	TO fun_cluster_worker
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."projects" TO "fun_cluster_worker";

/* Hazards:
 - ACQUIRES_ACCESS_EXCLUSIVE_LOCK: Index drops will lock out all accesses to the table. They should be fast.
 - INDEX_DROPPED: Dropping this index means queries that use this index might perform worse because they will no longer will be able to leverage it.
*/
DROP INDEX "tenant"."pgschemadiff_tmpidx_cluster_outbox_idx_c_UxODa3TTT_Sk3eQpqhLY1g";

DROP TRIGGER "cluster_reset_synced" ON "tenant"."clusters";

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For drops, this means you need to ensure that all functions this function depends on are dropped after this statement.
*/
DROP FUNCTION "tenant"."cluster_reset_synced"();

DROP TRIGGER "cluster_sync_notify" ON "tenant"."clusters";

ALTER TABLE "tenant"."clusters" ADD COLUMN "outbox_error" text COLLATE "pg_catalog"."default";

ALTER TABLE "tenant"."clusters" ADD COLUMN "outbox_retries" integer DEFAULT 0 NOT NULL;

ALTER TABLE "tenant"."clusters" ADD COLUMN "outbox_status" text COLLATE "pg_catalog"."default";

/* Hazards:
 - DELETES_DATA: Deletes all values in the column
*/
ALTER TABLE "tenant"."clusters" DROP COLUMN "sync_attempts";

/* Hazards:
 - DELETES_DATA: Deletes all values in the column
*/
ALTER TABLE "tenant"."clusters" DROP COLUMN "sync_claimed_at";

/* Hazards:
 - DELETES_DATA: Deletes all values in the column
*/
ALTER TABLE "tenant"."clusters" DROP COLUMN "sync_error";

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For drops, this means you need to ensure that all functions this function depends on are dropped after this statement.
*/
DROP FUNCTION "tenant"."cluster_sync_notify"();

DROP TRIGGER "node_pool_reset_cluster_synced" ON "tenant"."node_pools";

CREATE TRIGGER node_pool_outbox AFTER INSERT OR DELETE OR UPDATE OF name, machine_type, autoscale_min, autoscale_max, deleted ON tenant.node_pools FOR EACH ROW EXECUTE FUNCTION tenant.node_pool_outbox_trigger();

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For drops, this means you need to ensure that all functions this function depends on are dropped after this statement.
*/
DROP FUNCTION "tenant"."node_pool_reset_cluster_synced"();


-- Statements generated automatically, please review:
ALTER FUNCTION tenant.cluster_outbox_update_cluster_status() OWNER TO fun_owner;
ALTER FUNCTION tenant.node_pool_outbox_trigger() OWNER TO fun_owner;
