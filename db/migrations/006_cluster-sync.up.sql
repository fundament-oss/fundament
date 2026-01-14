SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.cluster_sync_create_on_insert()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    INSERT INTO tenant.cluster_sync (cluster_id)
    VALUES (NEW.id);
    RETURN NEW;
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.cluster_sync_notify()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    IF NEW.synced IS NULL THEN
        PERFORM pg_notify('cluster_sync', NEW.cluster_id::text);
    END IF;
    RETURN NEW;
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.cluster_sync_reset_on_delete()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    IF OLD.deleted IS NULL AND NEW.deleted IS NOT NULL THEN
        UPDATE tenant.cluster_sync
        SET synced = NULL
        WHERE cluster_id = NEW.id;
    END IF;
    RETURN NEW;
END;
$function$
;

CREATE TABLE "tenant"."cluster_sync" (
	"cluster_id" uuid NOT NULL,
	"synced" timestamp with time zone,
	"sync_error" text COLLATE "pg_catalog"."default",
	"sync_attempts" integer DEFAULT 0 NOT NULL,
	"sync_last_attempt" timestamp with time zone,
	"shoot_status" text COLLATE "pg_catalog"."default",
	"shoot_status_message" text COLLATE "pg_catalog"."default",
	"shoot_status_updated" timestamp with time zone
);

CREATE UNIQUE INDEX cluster_sync_pk ON tenant.cluster_sync USING btree (cluster_id);

ALTER TABLE "tenant"."cluster_sync" ADD CONSTRAINT "cluster_sync_pk" PRIMARY KEY USING INDEX "cluster_sync_pk";

CREATE INDEX cluster_sync_idx_status_check ON tenant.cluster_sync USING btree (shoot_status_updated) WHERE (synced IS NOT NULL);

CREATE INDEX cluster_sync_idx_unsynced ON tenant.cluster_sync USING btree (cluster_id) WHERE (synced IS NULL);

CREATE TRIGGER cluster_sync_notify AFTER INSERT OR UPDATE OF synced ON tenant.cluster_sync FOR EACH ROW EXECUTE FUNCTION tenant.cluster_sync_notify();

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "cluster_worker_all_access" ON "tenant"."clusters"
	AS PERMISSIVE
	FOR ALL
	TO fun_cluster_worker
	USING (true);

ALTER TABLE "tenant"."clusters" DROP CONSTRAINT "clusters_ck_status";

/* Hazards:
 - DELETES_DATA: Deletes all values in the column
*/
ALTER TABLE "tenant"."clusters" DROP COLUMN "status";

CREATE TRIGGER cluster_sync_create AFTER INSERT ON tenant.clusters FOR EACH ROW EXECUTE FUNCTION tenant.cluster_sync_create_on_insert();

CREATE TRIGGER cluster_sync_reset_on_delete AFTER UPDATE OF deleted ON tenant.clusters FOR EACH ROW EXECUTE FUNCTION tenant.cluster_sync_reset_on_delete();

ALTER TABLE "tenant"."cluster_sync" ADD CONSTRAINT "cluster_sync_fk_cluster" FOREIGN KEY (cluster_id) REFERENCES tenant.clusters(id) ON DELETE CASCADE NOT VALID;

ALTER TABLE "tenant"."cluster_sync" VALIDATE CONSTRAINT "cluster_sync_fk_cluster";

