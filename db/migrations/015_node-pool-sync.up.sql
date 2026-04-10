SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.cluster_outbox_update_cluster_status()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
DECLARE
    resolved_cluster_id uuid;
BEGIN
    IF NEW.cluster_id IS NOT NULL THEN
        UPDATE tenant.clusters
        SET outbox_status = NEW.status,
            outbox_retries = NEW.retries,
            outbox_error = NEW.status_info
        WHERE tenant.clusters.id = NEW.cluster_id;
    ELSIF NEW.node_pool_id IS NOT NULL THEN
        SELECT tenant.node_pools.cluster_id INTO resolved_cluster_id
        FROM tenant.node_pools
        WHERE tenant.node_pools.id = NEW.node_pool_id;

        IF resolved_cluster_id IS NOT NULL THEN
            UPDATE tenant.clusters
            SET outbox_status = NEW.status,
                outbox_retries = NEW.retries,
                outbox_error = NEW.status_info
            WHERE tenant.clusters.id = resolved_cluster_id;
        END IF;
    END IF;
    RETURN NULL;
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.clusters_tr_verify_deleted()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
	IF NEW.deleted IS NOT NULL AND EXISTS (
		SELECT 1
		FROM tenant.projects
		WHERE
			cluster_id = NEW.id
			AND deleted IS NULL
	) THEN
		RAISE EXCEPTION 'Cannot delete cluster with undeleted projects';
	END IF;
	RETURN NEW;
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.node_pool_outbox_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 SECURITY DEFINER COST 1
AS $function$
BEGIN
    INSERT INTO tenant.cluster_outbox (node_pool_id, event, source)
    VALUES (
        COALESCE(NEW.id, OLD.id),
        CASE
            WHEN TG_OP = 'INSERT' THEN 'created'
            WHEN OLD.deleted IS NULL AND NEW.deleted IS NOT NULL THEN 'deleted'
            ELSE 'updated'
        END,
        'trigger'
    );
    RETURN NULL;
END;
$function$
;

ALTER TABLE "tenant"."cluster_outbox" ADD COLUMN "deferrals" integer DEFAULT 0 NOT NULL;

ALTER TABLE "tenant"."cluster_outbox" ADD COLUMN "node_pool_id" uuid;

ALTER TABLE "tenant"."cluster_outbox" DROP CONSTRAINT "cluster_outbox_ck_single_fk";

ALTER TABLE "tenant"."cluster_outbox" ADD CONSTRAINT "cluster_outbox_ck_single_fk" CHECK((num_nonnulls(cluster_id, organization_user_id, project_member_id, node_pool_id) = 1)) NOT VALID;

ALTER TABLE "tenant"."cluster_outbox" VALIDATE CONSTRAINT "cluster_outbox_ck_single_fk";

ALTER TABLE "tenant"."cluster_outbox" DROP CONSTRAINT "cluster_outbox_ck_source";

ALTER TABLE "tenant"."cluster_outbox" ADD CONSTRAINT "cluster_outbox_ck_source" CHECK((source = ANY (ARRAY['trigger'::text, 'reconcile'::text, 'manual'::text, 'status'::text]))) NOT VALID;

ALTER TABLE "tenant"."cluster_outbox" VALIDATE CONSTRAINT "cluster_outbox_ck_source";

/* Hazards:
 - ACQUIRES_SHARE_LOCK: Non-concurrent index creates will lock out writes to the table during the duration of the index build.
*/
CREATE INDEX cluster_outbox_idx_node_pool_id ON tenant.cluster_outbox USING btree (node_pool_id, id DESC NULLS LAST);

/* Hazards:
 - ACQUIRES_SHARE_LOCK: Non-concurrent index creates will lock out writes to the table during the duration of the index build.
*/
CREATE INDEX node_pools_idx_cluster_id ON tenant.node_pools USING btree (cluster_id);

CREATE OR REPLACE TRIGGER node_pool_outbox AFTER INSERT OR UPDATE OF name, machine_type, autoscale_min, autoscale_max, deleted ON tenant.node_pools FOR EACH ROW EXECUTE FUNCTION tenant.node_pool_outbox_trigger();

ALTER TABLE "tenant"."cluster_outbox" ADD CONSTRAINT "cluster_outbox_fk_node_pool" FOREIGN KEY (node_pool_id) REFERENCES tenant.node_pools(id) NOT VALID;

ALTER TABLE "tenant"."cluster_outbox" VALIDATE CONSTRAINT "cluster_outbox_fk_node_pool";

