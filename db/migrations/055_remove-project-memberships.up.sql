SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.organizations_users_tr_remove_project_memberships()
 RETURNS trigger
 LANGUAGE plpgsql
 SECURITY DEFINER COST 1
AS $function$
BEGIN
    -- A user who leaves the organization leaves its projects too: a project
    -- membership left behind keeps its RBAC on the clusters and comes back into
    -- effect when the user is invited again. Every writer that revokes a
    -- membership is covered, the organization-api and funops alike. The
    -- protect_last_admin trigger refuses this while the user is the only admin
    -- of a project.
    -- Deleted projects are skipped: their memberships grant nothing, and
    -- protect_last_admin would refuse removing their only admin.
    -- The query takes a fresh snapshot, so it also sees a project membership
    -- added by a transaction this row update waited for.
    IF OLD.deleted IS NULL AND NEW.deleted IS NOT NULL THEN
        UPDATE tenant.project_members
        SET deleted = now()
        WHERE tenant.project_members.user_id = NEW.user_id
            AND tenant.project_members.deleted IS NULL
            AND tenant.project_members.project_id IN (
                SELECT tenant.projects.id
                FROM tenant.projects
                JOIN tenant.clusters ON tenant.clusters.id = tenant.projects.cluster_id
                WHERE tenant.clusters.organization_id = NEW.organization_id
                    AND tenant.projects.deleted IS NULL
            );
    END IF;
    RETURN NEW;
END;
$function$
;

CREATE TRIGGER remove_project_memberships AFTER UPDATE ON tenant.organizations_users FOR EACH ROW EXECUTE FUNCTION tenant.organizations_users_tr_remove_project_memberships();


-- Statements generated automatically, please review:
ALTER FUNCTION tenant.organizations_users_tr_remove_project_memberships() OWNER TO fun_owner;
