SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

CREATE OR REPLACE FUNCTION authn.current_organization_id()
 RETURNS uuid
 LANGUAGE sql
 STABLE PARALLEL SAFE COST 1
AS $function$
SELECT current_setting('app.current_organization_id')::uuid
$function$
;

CREATE OR REPLACE FUNCTION authn.current_user_id()
 RETURNS uuid
 LANGUAGE sql
 STABLE PARALLEL SAFE COST 1
AS $function$
SELECT current_setting('app.current_user_id')::uuid
$function$
;

CREATE OR REPLACE FUNCTION authn.is_cluster_in_organization(p_cluster_id uuid)
 RETURNS boolean
 LANGUAGE sql
 STABLE PARALLEL SAFE SECURITY DEFINER COST 1
AS $function$
SELECT EXISTS (
    SELECT 1 FROM tenant.clusters
    WHERE id = p_cluster_id
    AND organization_id = authn.current_organization_id()
)
$function$
;

ALTER FUNCTION authn.is_cluster_in_organization(p_cluster_id uuid) OWNER TO fun_authz;

CREATE OR REPLACE FUNCTION authn.is_project_in_organization(p_project_id uuid)
 RETURNS boolean
 LANGUAGE sql
 STABLE PARALLEL SAFE SECURITY DEFINER COST 1
AS $function$
SELECT EXISTS (
    SELECT 1 FROM tenant.projects
    WHERE id = p_project_id
    AND organization_id = authn.current_organization_id()
)
$function$
;

ALTER FUNCTION authn.is_project_in_organization(p_project_id uuid) OWNER TO fun_authz;

CREATE OR REPLACE FUNCTION authn.is_user_in_organization(p_user_id uuid)
 RETURNS boolean
 LANGUAGE sql
 STABLE PARALLEL SAFE SECURITY DEFINER COST 1
AS $function$
SELECT EXISTS (
    SELECT 1 FROM tenant.users
    WHERE id = p_user_id
    AND organization_id = authn.current_organization_id()
)
$function$
;

ALTER FUNCTION authn.is_user_in_organization(p_user_id uuid) OWNER TO fun_authz;

/* Hazards:
 - AUTHZ_UPDATE: Altering a policy could cause queries to fail if not correctly configured or allow unauthorized access to data.
*/
ALTER POLICY "organization_isolation" ON "tenant"."clusters"
	USING ((organization_id = authn.current_organization_id()));

/* Hazards:
 - AUTHZ_UPDATE: Altering a policy could cause queries to fail if not correctly configured or allow unauthorized access to data.
*/
ALTER POLICY "namespaces_organization_policy" ON "tenant"."namespaces"
	USING (authn.is_cluster_in_organization(cluster_id));

/* Hazards:
 - AUTHZ_UPDATE: Altering a policy could cause queries to fail if not correctly configured or allow unauthorized access to data.
*/
ALTER POLICY "node_pools_organization_policy" ON "tenant"."node_pools"
	USING (authn.is_cluster_in_organization(cluster_id));

CREATE TABLE "tenant"."project_members" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"project_id" uuid NOT NULL,
	"user_id" uuid NOT NULL,
	"role" text COLLATE "pg_catalog"."default" NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

ALTER TABLE tenant.project_members OWNER TO fun_owner;

ALTER TABLE "tenant"."project_members" ADD CONSTRAINT "project_members_ck_role" CHECK((role = ANY (ARRAY['admin'::text, 'viewer'::text])));

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

ALTER FUNCTION authn.is_project_member(p_project_id uuid, p_user_id uuid, p_role text) OWNER TO fun_authz;


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

ALTER FUNCTION tenant.project_has_members(p_project_id uuid) OWNER TO fun_authz;

CREATE POLICY "project_members_select_policy" ON "tenant"."project_members"
	AS PERMISSIVE
	FOR SELECT
	TO fun_fundament_api
    USING ((authn.is_project_in_organization(project_id) AND (authn.is_project_member(project_id, authn.current_user_id(), NULL::text) OR (user_id = authn.current_user_id()))));

CREATE POLICY "project_members_insert_policy" ON "tenant"."project_members"
    AS PERMISSIVE
    FOR INSERT
    TO fun_fundament_api
    WITH CHECK ((authn.is_project_in_organization(project_id) AND ((deleted IS NOT NULL) OR authn.is_user_in_organization(user_id)) AND (authn.is_project_member(project_id, authn.current_user_id(), 'admin'::text) OR (NOT tenant.project_has_members(project_id)))));

CREATE POLICY "project_members_update_policy" ON "tenant"."project_members"
    AS PERMISSIVE
    FOR UPDATE
    TO fun_fundament_api
    USING ((authn.is_project_in_organization(project_id) AND authn.is_project_member(project_id, authn.current_user_id(), 'admin'::text)));

ALTER TABLE "tenant"."project_members" ENABLE ROW LEVEL SECURITY;

CREATE UNIQUE INDEX project_members_pk ON tenant.project_members USING btree (id);

ALTER TABLE "tenant"."project_members" ADD CONSTRAINT "project_members_pk" PRIMARY KEY USING INDEX "project_members_pk";

CREATE UNIQUE INDEX project_members_uq_project_user ON tenant.project_members USING btree (project_id, user_id, deleted) NULLS NOT DISTINCT;

ALTER TABLE "tenant"."project_members" ADD CONSTRAINT "project_members_uq_project_user" UNIQUE USING INDEX "project_members_uq_project_user";

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.project_members_tr_protect_last_admin()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
DECLARE
    admin_count integer;
BEGIN
    -- Only check if we're soft-deleting an admin or demoting an admin (UPDATE role from admin)
    IF OLD.role = 'admin' AND OLD.deleted IS NULL THEN
        -- Check if this is a soft delete (setting deleted) or role demotion
        IF (NEW.deleted IS NOT NULL) OR (NEW.role != 'admin') THEN
            SELECT COUNT(*) INTO admin_count
            FROM tenant.project_members
            WHERE project_id = OLD.project_id
            AND role = 'admin'
            AND id != OLD.id
            AND deleted IS NULL;

            IF admin_count = 0 THEN
                RAISE EXCEPTION 'Cannot remove or demote the last admin of a project'
                            USING HINT = 'project_contains_one_admin';
            END IF;
        END IF;
    END IF;

    RETURN NEW;
END;
$function$
;

CREATE OR REPLACE TRIGGER protect_last_admin BEFORE UPDATE ON tenant.project_members FOR EACH ROW EXECUTE FUNCTION tenant.project_members_tr_protect_last_admin();

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "projects_delete_policy" ON "tenant"."projects"
	AS PERMISSIVE
	FOR DELETE
	TO fun_fundament_api
	USING (((organization_id = authn.current_organization_id()) AND authn.is_project_member(id, authn.current_user_id(), 'admin'::text)));

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "projects_insert_policy" ON "tenant"."projects"
	AS PERMISSIVE
	FOR INSERT
	TO fun_fundament_api
	WITH CHECK ((organization_id = authn.current_organization_id()));

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "projects_select_policy" ON "tenant"."projects"
	AS PERMISSIVE
	FOR SELECT
	TO fun_fundament_api
	USING ((organization_id = authn.current_organization_id()));

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "projects_update_policy" ON "tenant"."projects"
	AS PERMISSIVE
	FOR UPDATE
	TO fun_fundament_api
	USING (((organization_id = authn.current_organization_id()) AND authn.is_project_member(id, authn.current_user_id(), 'admin'::text)));

ALTER TABLE "tenant"."project_members" ADD CONSTRAINT "project_members_fk_project" FOREIGN KEY (project_id) REFERENCES tenant.projects(id) NOT VALID;

ALTER TABLE "tenant"."project_members" VALIDATE CONSTRAINT "project_members_fk_project";

ALTER TABLE "tenant"."project_members" ADD CONSTRAINT "project_members_fk_user" FOREIGN KEY (user_id) REFERENCES tenant.users(id) NOT VALID;

ALTER TABLE "tenant"."project_members" VALIDATE CONSTRAINT "project_members_fk_user";

/* Hazards:
 - AUTHZ_UPDATE: Altering a policy could cause queries to fail if not correctly configured or allow unauthorized access to data.
*/
ALTER POLICY "install_organization_policy" ON "appstore"."installs"
	USING (authn.is_cluster_in_organization(cluster_id));


-- Statements generated automatically, please review:
ALTER FUNCTION authn.current_organization_id() OWNER TO fun_fundament_api;
ALTER FUNCTION authn.current_user_id() OWNER TO fun_fundament_api;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.projects_tr_require_admin()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM tenant.project_members
        WHERE project_id = NEW.id
        AND role = 'admin'
        AND deleted IS NULL
    ) THEN
        RAISE EXCEPTION 'Project must have at least one admin';
    END IF;
    RETURN NEW;
END;
$function$
;

CREATE CONSTRAINT TRIGGER require_admin AFTER INSERT ON tenant.projects DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION tenant.projects_tr_require_admin();

CREATE INDEX project_members_idx_project_id ON tenant.project_members USING btree (project_id);

/* Hazards:
 - AUTHZ_UPDATE: Removing a permissive policy could cause queries to fail if not correctly configured.
*/
DROP POLICY "projects_organization_isolation" ON "tenant"."projects";

GRANT USAGE
   ON SCHEMA tenant
   TO fun_authz;

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."clusters" TO "fun_authz";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."project_members" TO "fun_authz";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."projects" TO "fun_authz";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."users" TO "fun_authz";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "tenant"."project_members" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."project_members" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "tenant"."project_members" TO "fun_fundament_api";
