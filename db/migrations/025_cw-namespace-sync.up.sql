SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.namespace_outbox_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 SECURITY DEFINER COST 1
AS $function$
BEGIN
    INSERT INTO tenant.cluster_outbox (namespace_id, event, source)
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

ALTER TABLE "tenant"."cluster_outbox" ADD COLUMN "namespace_id" uuid;

ALTER TABLE "tenant"."cluster_outbox" DROP CONSTRAINT "cluster_outbox_ck_single_fk";

ALTER TABLE "tenant"."cluster_outbox" ADD CONSTRAINT "cluster_outbox_ck_single_fk" CHECK((num_nonnulls(cluster_id, organization_user_id, project_member_id, node_pool_id, namespace_id) = 1)) NOT VALID;

ALTER TABLE "tenant"."cluster_outbox" VALIDATE CONSTRAINT "cluster_outbox_ck_single_fk";

/* Hazards:
 - ACQUIRES_SHARE_LOCK: Non-concurrent index creates will lock out writes to the table during the duration of the index build.
*/
CREATE INDEX cluster_outbox_idx_namespace_id ON tenant.cluster_outbox USING btree (namespace_id, id DESC NULLS LAST);

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "namespaces_cluster_worker_policy" ON "tenant"."namespaces"
	AS PERMISSIVE
	FOR SELECT
	TO fun_cluster_worker
	USING (true);

CREATE TRIGGER namespace_outbox AFTER INSERT OR UPDATE OF name, deleted ON tenant.namespaces FOR EACH ROW EXECUTE FUNCTION tenant.namespace_outbox_trigger();

ALTER TABLE "tenant"."cluster_outbox" ADD CONSTRAINT "cluster_outbox_fk_namespace" FOREIGN KEY (namespace_id) REFERENCES tenant.namespaces(id) NOT VALID;

ALTER TABLE "tenant"."cluster_outbox" VALIDATE CONSTRAINT "cluster_outbox_fk_namespace";


-- Statements generated automatically, please review:
ALTER FUNCTION tenant.namespace_outbox_trigger() OWNER TO fun_owner;
