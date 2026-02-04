SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

CREATE SCHEMA "authz";

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.outbox_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO authz.outbox (aggregate_type, aggregate_id, event_type, payload)
        VALUES (TG_TABLE_NAME, NEW.id::text, 'created', to_jsonb(NEW));
    ELSIF TG_OP = 'UPDATE' THEN
        IF NEW.deleted IS NOT NULL AND OLD.deleted IS NULL THEN
            INSERT INTO authz.outbox (aggregate_type, aggregate_id, event_type, payload)
            VALUES (TG_TABLE_NAME, NEW.id::text, 'deleted', to_jsonb(NEW));
        ELSE
            INSERT INTO authz.outbox (aggregate_type, aggregate_id, event_type, payload)
            VALUES (TG_TABLE_NAME, NEW.id::text, 'updated', to_jsonb(NEW));
        END IF;
    ELSIF TG_OP = 'DELETE' THEN
        INSERT INTO authz.outbox (aggregate_type, aggregate_id, event_type, payload)
        VALUES (TG_TABLE_NAME, OLD.id::text, 'deleted', to_jsonb(OLD));
    END IF;
    PERFORM pg_notify('authz_outbox', '');
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

ALTER INDEX "tenant"."organizations_uq_name" RENAME TO "pgschemadiff_tmpidx_organizations_uq_nam_BEZReNXkSeeA9_hjkAeIRQ";

CREATE TRIGGER api_keys_outbox AFTER INSERT OR DELETE OR UPDATE ON authn.api_keys FOR EACH ROW EXECUTE FUNCTION authz.outbox_trigger();

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

CREATE TRIGGER clusters_outbox AFTER INSERT OR DELETE OR UPDATE ON tenant.clusters FOR EACH ROW EXECUTE FUNCTION authz.outbox_trigger();

CREATE TRIGGER namespaces_outbox AFTER INSERT OR DELETE OR UPDATE ON tenant.namespaces FOR EACH ROW EXECUTE FUNCTION authz.outbox_trigger();

CREATE TRIGGER node_pools_outbox AFTER INSERT OR DELETE OR UPDATE ON tenant.node_pools FOR EACH ROW EXECUTE FUNCTION authz.outbox_trigger();

ALTER TABLE "tenant"."organizations" ADD COLUMN "deleted" timestamp with time zone;

/* Hazards:
 - ACQUIRES_SHARE_LOCK: Non-concurrent index creates will lock out writes to the table during the duration of the index build.
*/
CREATE UNIQUE INDEX organizations_uq_name ON tenant.organizations USING btree (name, deleted) NULLS NOT DISTINCT;

ALTER TABLE "tenant"."organizations" ADD CONSTRAINT "organizations_uq_name" UNIQUE USING INDEX "organizations_uq_name";

CREATE TRIGGER organizations_outbox AFTER INSERT OR DELETE OR UPDATE ON tenant.organizations FOR EACH ROW EXECUTE FUNCTION authz.outbox_trigger();

CREATE TRIGGER project_members_outbox AFTER INSERT OR DELETE OR UPDATE ON tenant.project_members FOR EACH ROW EXECUTE FUNCTION authz.outbox_trigger();

CREATE TRIGGER projects_outbox AFTER INSERT OR DELETE OR UPDATE ON tenant.projects FOR EACH ROW EXECUTE FUNCTION authz.outbox_trigger();

CREATE TRIGGER users_outbox AFTER INSERT OR DELETE OR UPDATE ON tenant.users FOR EACH ROW EXECUTE FUNCTION authz.outbox_trigger();

CREATE TRIGGER installs_outbox AFTER INSERT OR DELETE OR UPDATE ON zappstore.installs FOR EACH ROW EXECUTE FUNCTION authz.outbox_trigger();

/* Hazards:
 - ACQUIRES_ACCESS_EXCLUSIVE_LOCK: Index drops will lock out all accesses to the table. They should be fast.
 - INDEX_DROPPED: Dropping this index means queries that use this index might perform worse because they will no longer will be able to leverage it.
*/
ALTER TABLE "tenant"."organizations" DROP CONSTRAINT "pgschemadiff_tmpidx_organizations_uq_nam_BEZReNXkSeeA9_hjkAeIRQ";


-- Statements generated automatically, please review:
ALTER SCHEMA authz OWNER TO fun_owner;
ALTER FUNCTION authz.outbox_trigger() OWNER TO fun_owner;
ALTER TABLE authz.outbox OWNER TO fun_owner;
