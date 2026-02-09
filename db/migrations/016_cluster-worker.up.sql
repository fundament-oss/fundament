SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.cluster_reset_synced()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    NEW.synced := NULL;
    NEW.sync_claimed_at := NULL;
    NEW.sync_attempts := 0;
    NEW.sync_error := NULL;
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
    IF NEW.synced IS NULL AND (TG_OP = 'INSERT' OR OLD.synced IS NOT NULL) THEN
        PERFORM pg_notify('cluster_sync', '');
    END IF;
    RETURN NEW;
END;
$function$
;

CREATE TABLE "tenant"."cluster_events" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"cluster_id" uuid NOT NULL,
	"event_type" text COLLATE "pg_catalog"."default" NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"sync_action" text COLLATE "pg_catalog"."default",
	"message" text COLLATE "pg_catalog"."default",
	"attempt" integer
);

ALTER TABLE "tenant"."cluster_events" ADD CONSTRAINT "cluster_events_ck_event_type" CHECK((event_type = ANY (ARRAY['sync_requested'::text, 'sync_claimed'::text, 'sync_succeeded'::text, 'sync_failed'::text, 'status_progressing'::text, 'status_ready'::text, 'status_error'::text, 'status_deleted'::text])));

ALTER TABLE "tenant"."cluster_events" ADD CONSTRAINT "cluster_events_ck_sync_action" CHECK((sync_action = ANY (ARRAY['sync'::text, 'delete'::text])));

CREATE POLICY "cluster_events_organization_isolation" ON "tenant"."cluster_events"
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING ((EXISTS ( SELECT 1
   FROM tenant.clusters c
  WHERE ((c.id = cluster_events.cluster_id) AND (c.organization_id = authn.current_organization_id())))));

CREATE POLICY "cluster_events_worker_all_access" ON "tenant"."cluster_events"
	AS PERMISSIVE
	FOR ALL
	TO fun_cluster_worker
	USING (true);

ALTER TABLE "tenant"."cluster_events" ENABLE ROW LEVEL SECURITY;

GRANT INSERT ON "tenant"."cluster_events" TO "fun_cluster_worker";

GRANT SELECT ON "tenant"."cluster_events" TO "fun_cluster_worker";

GRANT INSERT ON "tenant"."cluster_events" TO "fun_fundament_api";

GRANT SELECT ON "tenant"."cluster_events" TO "fun_fundament_api";

CREATE UNIQUE INDEX cluster_events_pk ON tenant.cluster_events USING btree (id);

ALTER TABLE "tenant"."cluster_events" ADD CONSTRAINT "cluster_events_pk" PRIMARY KEY USING INDEX "cluster_events_pk";

CREATE INDEX cluster_events_idx_cluster_created ON tenant.cluster_events USING btree (cluster_id DESC NULLS LAST, created DESC NULLS LAST);

ALTER TABLE "tenant"."clusters" ADD COLUMN "shoot_status" text COLLATE "pg_catalog"."default";

ALTER TABLE "tenant"."clusters" ADD COLUMN "shoot_status_message" text COLLATE "pg_catalog"."default";

ALTER TABLE "tenant"."clusters" ADD COLUMN "shoot_status_updated" timestamp with time zone;

ALTER TABLE "tenant"."clusters" ADD COLUMN "sync_attempts" integer DEFAULT 0 NOT NULL;

ALTER TABLE "tenant"."clusters" ADD COLUMN "sync_claimed_at" timestamp with time zone;

ALTER TABLE "tenant"."clusters" ADD COLUMN "sync_error" text COLLATE "pg_catalog"."default";

ALTER TABLE "tenant"."clusters" ADD COLUMN "synced" timestamp with time zone;

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "cluster_worker_all_access" ON "tenant"."clusters"
	AS PERMISSIVE
	FOR ALL
	TO fun_cluster_worker
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."clusters" TO "fun_cluster_worker";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "tenant"."clusters" TO "fun_cluster_worker";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT USAGE ON SCHEMA "tenant" TO "fun_cluster_worker";

ALTER TABLE "tenant"."clusters" DROP CONSTRAINT "clusters_ck_status";

/* Hazards:
 - DELETES_DATA: Deletes all values in the column
*/
ALTER TABLE "tenant"."clusters" DROP COLUMN "status";

/* Hazards:
 - ACQUIRES_SHARE_LOCK: Non-concurrent index creates will lock out writes to the table during the duration of the index build.
*/
CREATE INDEX clusters_idx_needs_sync ON tenant.clusters USING btree (created) WHERE (synced IS NULL);

CREATE TRIGGER cluster_reset_synced BEFORE UPDATE OF name, region, kubernetes_version, deleted ON tenant.clusters FOR EACH ROW WHEN (((old.name IS DISTINCT FROM new.name) OR (old.region IS DISTINCT FROM new.region) OR (old.kubernetes_version IS DISTINCT FROM new.kubernetes_version) OR ((old.deleted IS NULL) AND (new.deleted IS NOT NULL)))) EXECUTE FUNCTION tenant.cluster_reset_synced();

CREATE TRIGGER cluster_sync_notify AFTER INSERT OR UPDATE ON tenant.clusters FOR EACH ROW EXECUTE FUNCTION tenant.cluster_sync_notify();

ALTER TABLE "tenant"."cluster_events" ADD CONSTRAINT "cluster_events_fk_cluster" FOREIGN KEY (cluster_id) REFERENCES tenant.clusters(id) ON DELETE CASCADE NOT VALID;

ALTER TABLE "tenant"."cluster_events" VALIDATE CONSTRAINT "cluster_events_fk_cluster";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."organizations" TO "fun_cluster_worker";


-- Statements generated automatically, please review:
ALTER FUNCTION tenant.cluster_reset_synced() OWNER TO fun_owner;
ALTER FUNCTION tenant.cluster_sync_notify() OWNER TO fun_owner;
ALTER TABLE tenant.cluster_events OWNER TO fun_owner;
