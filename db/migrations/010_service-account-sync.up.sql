SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.cluster_outbox_organization_user_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
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
 COST 1
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

ALTER TABLE "tenant"."cluster_outbox" ADD COLUMN "organization_user_id" uuid;

ALTER TABLE "tenant"."cluster_outbox" ADD COLUMN "project_member_id" uuid;

ALTER TABLE "tenant"."cluster_outbox" DROP CONSTRAINT "cluster_outbox_ck_event";

ALTER TABLE "tenant"."cluster_outbox" ADD CONSTRAINT "cluster_outbox_ck_event" CHECK((event = ANY (ARRAY['created'::text, 'updated'::text, 'deleted'::text, 'reconcile'::text, 'ready'::text]))) NOT VALID;

ALTER TABLE "tenant"."cluster_outbox" VALIDATE CONSTRAINT "cluster_outbox_ck_event";

ALTER TABLE "tenant"."cluster_outbox" DROP CONSTRAINT "cluster_outbox_ck_single_fk";

ALTER TABLE "tenant"."cluster_outbox" ADD CONSTRAINT "cluster_outbox_ck_single_fk" CHECK((num_nonnulls(cluster_id, organization_user_id, project_member_id) = 1)) NOT VALID;

ALTER TABLE "tenant"."cluster_outbox" VALIDATE CONSTRAINT "cluster_outbox_ck_single_fk";

ALTER TABLE "tenant"."cluster_outbox" DROP CONSTRAINT "cluster_outbox_ck_source";

ALTER TABLE "tenant"."cluster_outbox" ADD CONSTRAINT "cluster_outbox_ck_source" CHECK((source = ANY (ARRAY['trigger'::text, 'reconcile'::text, 'manual'::text, 'node_pool'::text, 'status'::text]))) NOT VALID;

ALTER TABLE "tenant"."cluster_outbox" VALIDATE CONSTRAINT "cluster_outbox_ck_source";

ALTER TABLE "tenant"."clusters" ADD COLUMN "shoot_api_server_url" text COLLATE "pg_catalog"."default";

ALTER TABLE "tenant"."clusters" ADD COLUMN "shoot_ca_data" text COLLATE "pg_catalog"."default";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "clusters_authn_api_policy" ON "tenant"."clusters"
	AS PERMISSIVE
	FOR SELECT
	TO fun_authn_api
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."clusters" TO "fun_authn_api";

CREATE TRIGGER cluster_outbox_organization_user AFTER INSERT OR UPDATE ON tenant.organizations_users FOR EACH ROW EXECUTE FUNCTION tenant.cluster_outbox_organization_user_trigger();

ALTER TABLE "tenant"."cluster_outbox" ADD CONSTRAINT "cluster_outbox_fk_organization_user" FOREIGN KEY (organization_user_id) REFERENCES tenant.organizations_users(id) NOT VALID;

ALTER TABLE "tenant"."cluster_outbox" VALIDATE CONSTRAINT "cluster_outbox_fk_organization_user";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "project_members_authn_api_policy" ON "tenant"."project_members"
	AS PERMISSIVE
	FOR SELECT
	TO fun_authn_api
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."project_members" TO "fun_authn_api";

CREATE TRIGGER cluster_outbox_project_member AFTER INSERT OR UPDATE ON tenant.project_members FOR EACH ROW EXECUTE FUNCTION tenant.cluster_outbox_project_member_trigger();

ALTER TABLE "tenant"."cluster_outbox" ADD CONSTRAINT "cluster_outbox_fk_project_member" FOREIGN KEY (project_member_id) REFERENCES tenant.project_members(id) NOT VALID;

ALTER TABLE "tenant"."cluster_outbox" VALIDATE CONSTRAINT "cluster_outbox_fk_project_member";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "projects_authn_api_policy" ON "tenant"."projects"
	AS PERMISSIVE
	FOR SELECT
	TO fun_authn_api
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."projects" TO "fun_authn_api";


-- Statements generated automatically, please review:
ALTER FUNCTION tenant.cluster_outbox_organization_user_trigger() OWNER TO fun_owner;
ALTER FUNCTION tenant.cluster_outbox_project_member_trigger() OWNER TO fun_owner;
