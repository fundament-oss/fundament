SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.cluster_outbox_cluster_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 SECURITY DEFINER COST 1
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
CREATE OR REPLACE FUNCTION tenant.cluster_outbox_organization_user_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 SECURITY DEFINER COST 1
AS $function$
BEGIN
    IF TG_OP = 'INSERT' OR NEW IS DISTINCT FROM OLD THEN
        INSERT INTO tenant.cluster_outbox (organization_user_id, event, source)
        VALUES (
            COALESCE(NEW.id, OLD.id),
            CASE
                WHEN TG_OP = 'INSERT' THEN 'created'
                WHEN OLD.deleted IS NULL AND NEW.deleted IS NOT NULL THEN 'deleted'
                ELSE 'updated'
            END,
            'trigger'
        );
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.cluster_outbox_project_member_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 SECURITY DEFINER COST 1
AS $function$
BEGIN
    IF TG_OP = 'INSERT' OR NEW IS DISTINCT FROM OLD THEN
        INSERT INTO tenant.cluster_outbox (project_member_id, event, source)
        VALUES (
            COALESCE(NEW.id, OLD.id),
            CASE
                WHEN TG_OP = 'INSERT' THEN 'created'
                WHEN OLD.deleted IS NULL AND NEW.deleted IS NOT NULL THEN 'deleted'
                ELSE 'updated'
            END,
            'trigger'
        );
    END IF;
    RETURN COALESCE(NEW, OLD);
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

ALTER TABLE "tenant"."cluster_events" DROP CONSTRAINT "cluster_events_ck_event_type";

ALTER TABLE "tenant"."cluster_events" ADD CONSTRAINT "cluster_events_ck_event_type" CHECK((event_type = ANY (ARRAY['sync_requested'::text, 'sync_claimed'::text, 'sync_succeeded'::text, 'sync_failed'::text, 'status_progressing'::text, 'status_ready'::text, 'status_error'::text, 'status_deleted'::text, 'user_sync_succeeded'::text, 'user_sync_failed'::text]))) NOT VALID;

ALTER TABLE "tenant"."cluster_events" VALIDATE CONSTRAINT "cluster_events_ck_event_type";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "organizations_users_cluster_worker_policy" ON "tenant"."organizations_users"
	AS PERMISSIVE
	FOR SELECT
	TO fun_cluster_worker
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."organizations_users" TO "fun_cluster_worker";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "project_members_cluster_worker_policy" ON "tenant"."project_members"
	AS PERMISSIVE
	FOR SELECT
	TO fun_cluster_worker
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."project_members" TO "fun_cluster_worker";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "users_cluster_worker_policy" ON "tenant"."users"
	AS PERMISSIVE
	FOR SELECT
	TO fun_cluster_worker
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."users" TO "fun_cluster_worker";

