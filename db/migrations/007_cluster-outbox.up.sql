SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

CREATE OR REPLACE FUNCTION authn.is_project_member(p_project_id uuid, p_user_id uuid, p_role text)
 RETURNS boolean
 LANGUAGE sql
 STABLE PARALLEL SAFE SECURITY DEFINER COST 1
AS $function$
SELECT EXISTS (
    SELECT 1 FROM tenant.project_members
    WHERE project_id = p_project_id
    AND user_id = p_user_id
    AND (p_role IS NULL OR role = p_role)
    AND deleted IS NULL
)
$function$
;

CREATE OR REPLACE FUNCTION authn.is_user_in_organization(p_user_id uuid)
 RETURNS boolean
 LANGUAGE sql
 STABLE PARALLEL SAFE SECURITY DEFINER COST 1
AS $function$
SELECT EXISTS (
    SELECT 1 FROM tenant.organizations_users
    WHERE user_id = p_user_id
    AND organization_id = authn.current_organization_id()
    AND deleted IS NULL
)
$function$
;

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
CREATE OR REPLACE FUNCTION tenant.cluster_outbox_notify()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    PERFORM pg_notify('cluster_outbox', '');
    RETURN NEW;
END;
$function$
;

CREATE OR REPLACE FUNCTION tenant.project_has_members(p_project_id uuid)
 RETURNS boolean
 LANGUAGE sql
 STABLE PARALLEL SAFE SECURITY DEFINER COST 1
AS $function$
SELECT EXISTS (
    SELECT 1 FROM tenant.project_members
    WHERE project_id = p_project_id
    AND deleted IS NULL
)
$function$
;

/* Hazards:
 - AUTHZ_UPDATE: Altering a policy could cause queries to fail if not correctly configured or allow unauthorized access to data.
*/
ALTER POLICY "api_keys_organization_policy" ON "authn"."api_keys"
	USING (((organization_id = (current_setting('app.current_organization_id'::text))::uuid) AND (user_id = (current_setting('app.current_user_id'::text))::uuid)));

CREATE TABLE "tenant"."cluster_outbox" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"cluster_id" uuid,
	"event" text COLLATE "pg_catalog"."default" DEFAULT 'updated'::text NOT NULL,
	"source" text COLLATE "pg_catalog"."default" DEFAULT 'trigger'::text NOT NULL,
	"status" text COLLATE "pg_catalog"."default" DEFAULT 'pending'::text NOT NULL,
	"retries" integer DEFAULT 0 NOT NULL,
	"retry_after" timestamp with time zone,
	"processed" timestamp with time zone,
	"failed" timestamp with time zone,
	"status_info" text COLLATE "pg_catalog"."default",
	"created" timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE "tenant"."cluster_outbox" ADD CONSTRAINT "cluster_outbox_ck_event" CHECK((event = ANY (ARRAY['created'::text, 'updated'::text, 'deleted'::text, 'reconcile'::text])));

ALTER TABLE "tenant"."cluster_outbox" ADD CONSTRAINT "cluster_outbox_ck_single_fk" CHECK((num_nonnulls(cluster_id) = 1));

ALTER TABLE "tenant"."cluster_outbox" ADD CONSTRAINT "cluster_outbox_ck_source" CHECK((source = ANY (ARRAY['trigger'::text, 'reconcile'::text, 'manual'::text])));

ALTER TABLE "tenant"."cluster_outbox" ADD CONSTRAINT "cluster_outbox_ck_status" CHECK((status = ANY (ARRAY['pending'::text, 'completed'::text, 'retrying'::text, 'failed'::text])));

GRANT INSERT ON "tenant"."cluster_outbox" TO "fun_cluster_worker";

GRANT SELECT ON "tenant"."cluster_outbox" TO "fun_cluster_worker";

GRANT UPDATE ON "tenant"."cluster_outbox" TO "fun_cluster_worker";

GRANT INSERT ON "tenant"."cluster_outbox" TO "fun_fundament_api";

CREATE UNIQUE INDEX cluster_outbox_pk ON tenant.cluster_outbox USING btree (id);

ALTER TABLE "tenant"."cluster_outbox" ADD CONSTRAINT "cluster_outbox_pk" PRIMARY KEY USING INDEX "cluster_outbox_pk";

CREATE INDEX cluster_outbox_idx_cluster_id ON tenant.cluster_outbox USING btree (cluster_id);

CREATE INDEX cluster_outbox_status_retry_idx ON tenant.cluster_outbox USING btree (status, retry_after, id);

CREATE TRIGGER cluster_outbox_notify AFTER INSERT ON tenant.cluster_outbox FOR EACH ROW EXECUTE FUNCTION tenant.cluster_outbox_notify();

CREATE TRIGGER cluster_outbox_cluster AFTER INSERT OR UPDATE ON tenant.clusters FOR EACH ROW EXECUTE FUNCTION tenant.cluster_outbox_cluster_trigger();

ALTER TABLE "tenant"."cluster_outbox" ADD CONSTRAINT "cluster_outbox_fk_cluster" FOREIGN KEY (cluster_id) REFERENCES tenant.clusters(id) NOT VALID;

ALTER TABLE "tenant"."cluster_outbox" VALIDATE CONSTRAINT "cluster_outbox_fk_cluster";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "organizations_cluster_worker_policy" ON "tenant"."organizations"
	AS PERMISSIVE
	FOR SELECT
	TO fun_cluster_worker
	USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Removing a permissive policy could cause queries to fail if not correctly configured.
*/
DROP POLICY "organizations_organization_policy" ON "tenant"."organizations";

/* Hazards:
 - AUTHZ_UPDATE: Removing a permissive policy could cause queries to fail if not correctly configured.
*/
DROP POLICY "organizations_user_select_policy" ON "tenant"."organizations";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "organizations_select_policy" ON "tenant"."organizations"
	AS PERMISSIVE
	FOR SELECT
	TO fun_fundament_api
	USING (authn.is_organization_member(id));

/* Hazards:
 - AUTHZ_UPDATE: Removing a permissive policy could cause queries to fail if not correctly configured.
*/
DROP POLICY "organizations_users_organization_policy" ON "tenant"."organizations_users";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "organizations_users_insert_policy" ON "tenant"."organizations_users"
	AS PERMISSIVE
	FOR INSERT
	TO fun_fundament_api
	WITH CHECK ((organization_id = authn.current_organization_id()));

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "organizations_users_select_policy" ON "tenant"."organizations_users"
	AS PERMISSIVE
	FOR SELECT
	TO fun_fundament_api
	USING ((organization_id = authn.current_organization_id()));

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "organizations_users_update_policy" ON "tenant"."organizations_users"
	AS PERMISSIVE
	FOR UPDATE
	TO fun_fundament_api
	USING ((organization_id = authn.current_organization_id()));

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
 - AUTHZ_UPDATE: Removing a permissive policy could cause queries to fail if not correctly configured.
*/
DROP POLICY "project_members_organization_policy" ON "tenant"."project_members";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "project_members_insert_policy" ON "tenant"."project_members"
	AS PERMISSIVE
	FOR INSERT
	TO fun_fundament_api
	WITH CHECK ((authn.is_project_in_organization(project_id) AND ((deleted IS NOT NULL) OR authn.is_user_in_organization(user_id)) AND (authn.is_project_member(project_id, authn.current_user_id(), 'admin'::text) OR (NOT tenant.project_has_members(project_id)))));

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "project_members_select_policy" ON "tenant"."project_members"
	AS PERMISSIVE
	FOR SELECT
	TO fun_fundament_api
	USING ((authn.is_project_in_organization(project_id) AND (authn.is_project_member(project_id, authn.current_user_id(), NULL::text) OR (user_id = authn.current_user_id()))));

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "project_members_update_policy" ON "tenant"."project_members"
	AS PERMISSIVE
	FOR UPDATE
	TO fun_fundament_api
	USING ((authn.is_project_in_organization(project_id) AND authn.is_project_member(project_id, authn.current_user_id(), 'admin'::text)));

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
 - AUTHZ_UPDATE: Removing a permissive policy could cause queries to fail if not correctly configured.
*/
DROP POLICY "projects_organization_policy" ON "tenant"."projects";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "projects_delete_policy" ON "tenant"."projects"
	AS PERMISSIVE
	FOR DELETE
	TO fun_fundament_api
	USING ((authn.is_cluster_in_organization(cluster_id) AND authn.is_project_member(id, authn.current_user_id(), 'admin'::text)));

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "projects_insert_policy" ON "tenant"."projects"
	AS PERMISSIVE
	FOR INSERT
	TO fun_fundament_api
	WITH CHECK (authn.is_cluster_in_organization(cluster_id));

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "projects_select_policy" ON "tenant"."projects"
	AS PERMISSIVE
	FOR SELECT
	TO fun_fundament_api
	USING (authn.is_cluster_in_organization(cluster_id));

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "projects_update_policy" ON "tenant"."projects"
	AS PERMISSIVE
	FOR UPDATE
	TO fun_fundament_api
	USING ((authn.is_cluster_in_organization(cluster_id) AND authn.is_project_member(id, authn.current_user_id(), 'admin'::text)));


-- Statements generated automatically, please review:
ALTER FUNCTION authn.is_project_member(p_project_id uuid, p_user_id uuid, p_role text) OWNER TO fun_authz;
ALTER FUNCTION authn.is_user_in_organization(p_user_id uuid) OWNER TO fun_authz;
ALTER FUNCTION tenant.cluster_outbox_cluster_trigger() OWNER TO fun_owner;
ALTER FUNCTION tenant.cluster_outbox_notify() OWNER TO fun_owner;
ALTER FUNCTION tenant.project_has_members(p_project_id uuid) OWNER TO fun_authz;
ALTER TABLE tenant.cluster_outbox OWNER TO fun_owner;
