SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.api_keys_sync_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    -- Only insert into outbox if this is an INSERT, DELETE, or if data actually changed
    IF TG_OP = 'INSERT' OR NEW IS DISTINCT FROM OLD THEN
        INSERT INTO authz.outbox (api_key_id)
        VALUES (COALESCE(NEW.id, OLD.id));
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.clusters_sync_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    -- Only insert into outbox if this is an INSERT or if data actually changed
    IF TG_OP = 'INSERT' OR NEW IS DISTINCT FROM OLD THEN
        INSERT INTO authz.outbox (cluster_id)
        VALUES (COALESCE(NEW.id, OLD.id));
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.installs_sync_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    -- Only insert into outbox if this is an INSERT or if data actually changed
    IF TG_OP = 'INSERT' OR NEW IS DISTINCT FROM OLD THEN
        INSERT INTO authz.outbox (install_id)
        VALUES (COALESCE(NEW.id, OLD.id));
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.namespaces_sync_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    -- Only insert into outbox if this is an INSERT or if data actually changed
    IF TG_OP = 'INSERT' OR NEW IS DISTINCT FROM OLD THEN
        INSERT INTO authz.outbox (namespace_id)
        VALUES (COALESCE(NEW.id, OLD.id));
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.node_pools_sync_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    -- Only insert into outbox if this is an INSERT or if data actually changed
    IF TG_OP = 'INSERT' OR NEW IS DISTINCT FROM OLD THEN
        INSERT INTO authz.outbox (node_pool_id)
        VALUES (COALESCE(NEW.id, OLD.id));
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.project_members_sync_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    -- Only insert into outbox if this is an INSERT or if data actually changed
    IF TG_OP = 'INSERT' OR NEW IS DISTINCT FROM OLD THEN
        INSERT INTO authz.outbox (project_member_id)
        VALUES (COALESCE(NEW.id, OLD.id));
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.projects_sync_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    -- Only insert into outbox if this is an INSERT or if data actually changed
    IF TG_OP = 'INSERT' OR NEW IS DISTINCT FROM OLD THEN
        INSERT INTO authz.outbox (project_id)
        VALUES (COALESCE(NEW.id, OLD.id));
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.users_sync_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    -- Only insert into outbox if this is an INSERT or if data actually changed
    IF TG_OP = 'INSERT' OR NEW IS DISTINCT FROM OLD THEN
        INSERT INTO authz.outbox (user_id)
        VALUES (COALESCE(NEW.id, OLD.id));
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

ALTER TABLE "tenant"."users" ADD CONSTRAINT "users_ck_role" CHECK((role = ANY (ARRAY['admin'::text, 'viewer'::text]))) NOT VALID;

ALTER TABLE "tenant"."users" VALIDATE CONSTRAINT "users_ck_role";
