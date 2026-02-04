SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

CREATE SCHEMA "authz";

GRANT USAGE
   ON SCHEMA authz
   TO fun_authn_api;

GRANT USAGE
   ON SCHEMA authz
   TO fun_fundament_api;

GRANT USAGE
   ON SCHEMA authz
   TO fun_authz_worker;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.project_members_outbox_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    IF TG_OP = 'INSERT' AND NEW.deleted IS NULL THEN
        INSERT INTO authz.outbox (aggregate_type, aggregate_id, event_type, payload)
        VALUES ('project_member', NEW.id::text, 'created', jsonb_build_object(
            'project_id', NEW.project_id,
            'user_id', NEW.user_id,
            'role', NEW.role
        ));
    ELSIF TG_OP = 'UPDATE' THEN
        IF OLD.deleted IS NULL AND NEW.deleted IS NOT NULL THEN
            INSERT INTO authz.outbox (aggregate_type, aggregate_id, event_type, payload)
            VALUES ('project_member', OLD.id::text, 'deleted', jsonb_build_object(
                'project_id', OLD.project_id,
                'user_id', OLD.user_id,
                'role', OLD.role
            ));
        ELSIF OLD.role IS DISTINCT FROM NEW.role THEN
            INSERT INTO authz.outbox (aggregate_type, aggregate_id, event_type, payload)
            VALUES ('project_member', NEW.id::text, 'role_changed', jsonb_build_object(
                'project_id', NEW.project_id,
                'user_id', NEW.user_id,
                'old_role', OLD.role,
                'new_role', NEW.role
            ));
        END IF;
    END IF;
    PERFORM pg_notify('authz_outbox', '');
    RETURN NEW;
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.projects_outbox_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO authz.outbox (aggregate_type, aggregate_id, event_type, payload)
        VALUES ('project', NEW.id::text, 'created', jsonb_build_object(
            'project_id', NEW.id,
            'organization_id', NEW.organization_id,
            'name', NEW.name
        ));
    ELSIF TG_OP = 'UPDATE' AND OLD.deleted IS NULL AND NEW.deleted IS NOT NULL THEN
        INSERT INTO authz.outbox (aggregate_type, aggregate_id, event_type, payload)
        VALUES ('project', OLD.id::text, 'deleted', jsonb_build_object(
            'project_id', OLD.id,
            'organization_id', OLD.organization_id,
            'name', OLD.name
        ));
    END IF;
    PERFORM pg_notify('authz_outbox', '');
    RETURN NEW;
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.users_outbox_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    IF TG_OP = 'INSERT' AND NEW.deleted IS NULL THEN
        INSERT INTO authz.outbox (aggregate_type, aggregate_id, event_type, payload)
        VALUES ('user', NEW.id::text, 'created', jsonb_build_object(
            'user_id', NEW.id,
            'organization_id', NEW.organization_id,
            'role', NEW.role
        ));
    ELSIF TG_OP = 'UPDATE' AND OLD.deleted IS NULL AND NEW.deleted IS NOT NULL THEN
        INSERT INTO authz.outbox (aggregate_type, aggregate_id, event_type, payload)
        VALUES ('user', OLD.id::text, 'deleted', jsonb_build_object(
            'user_id', OLD.id,
            'organization_id', OLD.organization_id,
            'role', OLD.role
        ));
    ELSIF TG_OP = 'UPDATE' AND OLD.role IS DISTINCT FROM NEW.role THEN
        INSERT INTO authz.outbox (aggregate_type, aggregate_id, event_type, payload)
        VALUES ('user', NEW.id::text, 'role_changed', jsonb_build_object(
            'user_id', NEW.id,
            'organization_id', NEW.organization_id,
            'old_role', OLD.role,
            'new_role', NEW.role
        ));
    END IF;
    PERFORM pg_notify('authz_outbox', '');
    RETURN NEW;
END;
$function$
;

CREATE TABLE "authz"."outbox" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"aggregate_type" text COLLATE "pg_catalog"."default" NOT NULL,
	"aggregate_id" text COLLATE "pg_catalog"."default" NOT NULL,
	"event_type" text COLLATE "pg_catalog"."default" NOT NULL,
	"payload" jsonb NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"processed_at" timestamp with time zone
);

GRANT INSERT ON "authz"."outbox" TO "fun_authn_api";

GRANT SELECT ON "authz"."outbox" TO "fun_authz_worker";

GRANT UPDATE ON "authz"."outbox" TO "fun_authz_worker";

GRANT INSERT ON "authz"."outbox" TO "fun_fundament_api";

CREATE UNIQUE INDEX outbox_pk ON authz.outbox USING btree (id);

ALTER TABLE "authz"."outbox" ADD CONSTRAINT "outbox_pk" PRIMARY KEY USING INDEX "outbox_pk";

CREATE INDEX outbox_idx_unprocessed ON authz.outbox USING btree (created_at) WHERE (processed_at IS NULL);

CREATE TRIGGER project_members_outbox AFTER INSERT OR UPDATE ON tenant.project_members FOR EACH ROW EXECUTE FUNCTION authz.project_members_outbox_trigger();

CREATE TRIGGER projects_outbox AFTER INSERT OR UPDATE ON tenant.projects FOR EACH ROW EXECUTE FUNCTION authz.projects_outbox_trigger();

CREATE TRIGGER users_outbox AFTER INSERT OR UPDATE ON tenant.users FOR EACH ROW EXECUTE FUNCTION authz.users_outbox_trigger();


-- Statements generated automatically, please review:
ALTER SCHEMA authz OWNER TO fun_owner;
ALTER FUNCTION authz.project_members_outbox_trigger() OWNER TO fun_owner;
ALTER FUNCTION authz.projects_outbox_trigger() OWNER TO fun_owner;
ALTER FUNCTION authz.users_outbox_trigger() OWNER TO fun_owner;
ALTER TABLE authz.outbox OWNER TO fun_owner;
