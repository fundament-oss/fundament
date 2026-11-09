SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

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
            -- Lock the project so concurrent removals or demotions of its admins
            -- take turns: each counts the admins only after the previous one has
            -- committed, and the last one is refused instead of both passing.
            PERFORM 1
            FROM tenant.projects
            WHERE id = OLD.project_id
            FOR NO KEY UPDATE;

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

