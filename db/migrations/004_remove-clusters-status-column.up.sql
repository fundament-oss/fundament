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

ALTER TABLE "tenant"."cluster_sync" DROP CONSTRAINT "cluster_sync_cluster_id_fkey";

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

/* Hazards:
 - ACQUIRES_ACCESS_EXCLUSIVE_LOCK: Index drops will lock out all accesses to the table. They should be fast.
 - INDEX_DROPPED: Dropping this index means queries that use this index might perform worse because they will no longer will be able to leverage it.
*/
ALTER TABLE "tenant"."cluster_sync" DROP CONSTRAINT "cluster_sync_pkey";

ALTER TABLE "tenant"."cluster_sync" ADD CONSTRAINT "cluster_sync_fk_cluster" FOREIGN KEY (cluster_id) REFERENCES tenant.clusters(id) ON DELETE CASCADE NOT VALID;

ALTER TABLE "tenant"."cluster_sync" VALIDATE CONSTRAINT "cluster_sync_fk_cluster";

/* Hazards:
 - ACQUIRES_SHARE_LOCK: Non-concurrent index creates will lock out writes to the table during the duration of the index build.
*/
CREATE UNIQUE INDEX cluster_sync_pk ON tenant.cluster_sync USING btree (cluster_id);

ALTER TABLE "tenant"."cluster_sync" ADD CONSTRAINT "cluster_sync_pk" PRIMARY KEY USING INDEX "cluster_sync_pk";

