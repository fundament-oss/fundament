SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

CREATE SCHEMA "authz";

GRANT USAGE
   ON SCHEMA authz
   TO fun_fundament_api;

GRANT USAGE
   ON SCHEMA authz
   TO fun_authn_api;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.api_keys_sync_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    INSERT INTO authz.outbox (api_key_id)
    VALUES (COALESCE(NEW.id, OLD.id));
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
    INSERT INTO authz.outbox (cluster_id)
    VALUES (COALESCE(NEW.id, OLD.id));
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
    INSERT INTO authz.outbox (install_id)
    VALUES (COALESCE(NEW.id, OLD.id));
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
    INSERT INTO authz.outbox (namespace_id)
    VALUES (COALESCE(NEW.id, OLD.id));
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
    INSERT INTO authz.outbox (node_pool_id)
    VALUES (COALESCE(NEW.id, OLD.id));
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.outbox_notify_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    PERFORM pg_notify('authz_outbox', '');
    RETURN NEW;
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
    INSERT INTO authz.outbox (project_member_id)
    VALUES (COALESCE(NEW.id, OLD.id));
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
    INSERT INTO authz.outbox (project_id)
    VALUES (COALESCE(NEW.id, OLD.id));
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
    INSERT INTO authz.outbox (user_id)
    VALUES (COALESCE(NEW.id, OLD.id));
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

ALTER INDEX "tenant"."organizations_uq_name" RENAME TO "pgschemadiff_tmpidx_organizations_uq_nam_75SClg18S2mT9vWHkD9SFw";

CREATE TABLE "authz"."outbox" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"user_id" uuid,
	"project_id" uuid,
	"project_member_id" uuid,
	"cluster_id" uuid,
	"node_pool_id" uuid,
	"namespace_id" uuid,
	"api_key_id" uuid,
	"install_id" uuid,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"processed" timestamp with time zone,
	"retries" integer DEFAULT 0 NOT NULL,
	"retry_after" timestamp with time zone,
	"failed" timestamp with time zone,
	status text NOT NULL DEFAULT 'pending',
	status_info text
);

ALTER TABLE "authz"."outbox" ADD CONSTRAINT "outbox_ck_single_fk" CHECK((num_nonnulls(user_id, project_id, project_member_id, cluster_id, node_pool_id, namespace_id, api_key_id, install_id) = 1));

ALTER TABLE "authz"."outbox" ADD CONSTRAINT "outbox_ck_status" CHECK((status = ANY (ARRAY['pending'::text, 'completed'::text, 'retrying'::text, 'failed'::text]))) NOT VALID;

ALTER TABLE "authz"."outbox" VALIDATE CONSTRAINT "outbox_ck_status";

GRANT INSERT ON "authz"."outbox" TO "fun_authn_api";

GRANT INSERT ON "authz"."outbox" TO "fun_fundament_api";

ALTER TABLE "authz"."outbox" ADD CONSTRAINT "outbox_fk_api_key" FOREIGN KEY (api_key_id) REFERENCES authn.api_keys(id) NOT VALID;

ALTER TABLE "authz"."outbox" VALIDATE CONSTRAINT "outbox_fk_api_key";

CREATE TRIGGER api_keys_outbox AFTER INSERT OR UPDATE ON authn.api_keys FOR EACH ROW EXECUTE FUNCTION authz.api_keys_sync_trigger();

CREATE UNIQUE INDEX outbox_pk ON authz.outbox USING btree (id);

ALTER TABLE "authz"."outbox" ADD CONSTRAINT "outbox_pk" PRIMARY KEY USING INDEX "outbox_pk";

CREATE INDEX outbox_idx_unprocessed ON authz.outbox USING btree (created) WHERE (processed IS NULL);

CREATE TRIGGER outbox_notify AFTER INSERT ON authz.outbox FOR EACH ROW EXECUTE FUNCTION authz.outbox_notify_trigger();

CREATE TRIGGER clusters_outbox AFTER INSERT OR UPDATE ON tenant.clusters FOR EACH ROW EXECUTE FUNCTION authz.clusters_sync_trigger();

ALTER TABLE "authz"."outbox" ADD CONSTRAINT "outbox_fk_cluster" FOREIGN KEY (cluster_id) REFERENCES tenant.clusters(id) NOT VALID;

ALTER TABLE "authz"."outbox" VALIDATE CONSTRAINT "outbox_fk_cluster";

CREATE TRIGGER namespaces_outbox AFTER INSERT OR UPDATE ON tenant.namespaces FOR EACH ROW EXECUTE FUNCTION authz.namespaces_sync_trigger();

ALTER TABLE "authz"."outbox" ADD CONSTRAINT "outbox_fk_namespace" FOREIGN KEY (namespace_id) REFERENCES tenant.namespaces(id) NOT VALID;

ALTER TABLE "authz"."outbox" VALIDATE CONSTRAINT "outbox_fk_namespace";

CREATE TRIGGER node_pools_outbox AFTER INSERT OR UPDATE ON tenant.node_pools FOR EACH ROW EXECUTE FUNCTION authz.node_pools_sync_trigger();

ALTER TABLE "authz"."outbox" ADD CONSTRAINT "outbox_fk_node_pool" FOREIGN KEY (node_pool_id) REFERENCES tenant.node_pools(id) NOT VALID;

ALTER TABLE "authz"."outbox" VALIDATE CONSTRAINT "outbox_fk_node_pool";

ALTER TABLE "tenant"."organizations" ADD COLUMN "deleted" timestamp with time zone;

/* Hazards:
 - ACQUIRES_SHARE_LOCK: Non-concurrent index creates will lock out writes to the table during the duration of the index build.
*/
CREATE UNIQUE INDEX organizations_uq_name ON tenant.organizations USING btree (name, deleted) NULLS NOT DISTINCT;

ALTER TABLE "tenant"."organizations" ADD CONSTRAINT "organizations_uq_name" UNIQUE USING INDEX "organizations_uq_name";

CREATE TRIGGER project_members_outbox AFTER INSERT OR UPDATE ON tenant.project_members FOR EACH ROW EXECUTE FUNCTION authz.project_members_sync_trigger();

ALTER TABLE "authz"."outbox" ADD CONSTRAINT "outbox_fk_project_member" FOREIGN KEY (project_member_id) REFERENCES tenant.project_members(id) NOT VALID;

ALTER TABLE "authz"."outbox" VALIDATE CONSTRAINT "outbox_fk_project_member";

CREATE TRIGGER projects_outbox AFTER INSERT OR UPDATE ON tenant.projects FOR EACH ROW EXECUTE FUNCTION authz.projects_sync_trigger();

ALTER TABLE "authz"."outbox" ADD CONSTRAINT "outbox_fk_project" FOREIGN KEY (project_id) REFERENCES tenant.projects(id) NOT VALID;

ALTER TABLE "authz"."outbox" VALIDATE CONSTRAINT "outbox_fk_project";

CREATE TRIGGER users_outbox AFTER INSERT OR UPDATE ON tenant.users FOR EACH ROW EXECUTE FUNCTION authz.users_sync_trigger();

ALTER TABLE "authz"."outbox" ADD CONSTRAINT "outbox_fk_user" FOREIGN KEY (user_id) REFERENCES tenant.users(id) NOT VALID;

ALTER TABLE "authz"."outbox" VALIDATE CONSTRAINT "outbox_fk_user";

CREATE TRIGGER installs_outbox AFTER INSERT OR UPDATE ON zappstore.installs FOR EACH ROW EXECUTE FUNCTION authz.installs_sync_trigger();

ALTER TABLE "authz"."outbox" ADD CONSTRAINT "outbox_fk_install" FOREIGN KEY (install_id) REFERENCES zappstore.installs(id) NOT VALID;

ALTER TABLE "authz"."outbox" VALIDATE CONSTRAINT "outbox_fk_install";

/* Hazards:
 - ACQUIRES_ACCESS_EXCLUSIVE_LOCK: Index drops will lock out all accesses to the table. They should be fast.
 - INDEX_DROPPED: Dropping this index means queries that use this index might perform worse because they will no longer will be able to leverage it.
*/
ALTER TABLE "tenant"."organizations" DROP CONSTRAINT "pgschemadiff_tmpidx_organizations_uq_nam_75SClg18S2mT9vWHkD9SFw";


-- Statements generated automatically, please review:
ALTER SCHEMA authz OWNER TO fun_owner;
ALTER FUNCTION authz.api_keys_sync_trigger() OWNER TO fun_owner;
ALTER FUNCTION authz.clusters_sync_trigger() OWNER TO fun_owner;
ALTER FUNCTION authz.installs_sync_trigger() OWNER TO fun_owner;
ALTER FUNCTION authz.namespaces_sync_trigger() OWNER TO fun_owner;
ALTER FUNCTION authz.node_pools_sync_trigger() OWNER TO fun_owner;
ALTER FUNCTION authz.outbox_notify_trigger() OWNER TO fun_owner;
ALTER FUNCTION authz.project_members_sync_trigger() OWNER TO fun_owner;
ALTER FUNCTION authz.projects_sync_trigger() OWNER TO fun_owner;
ALTER FUNCTION authz.users_sync_trigger() OWNER TO fun_owner;
ALTER TABLE authz.outbox OWNER TO fun_owner;

-- Grants for fun_authz_worker to process the outbox
GRANT USAGE ON SCHEMA authz TO fun_authz_worker;
GRANT USAGE ON SCHEMA tenant TO fun_authz_worker;
GRANT USAGE ON SCHEMA authn TO fun_authz_worker;
GRANT USAGE ON SCHEMA zappstore TO fun_authz_worker;

GRANT SELECT ON public.schema_migrations TO fun_authz_worker;

GRANT SELECT, UPDATE ON authz.outbox TO fun_authz_worker;

GRANT SELECT ON tenant.users TO fun_authz_worker;
GRANT SELECT ON tenant.projects TO fun_authz_worker;
GRANT SELECT ON tenant.project_members TO fun_authz_worker;
GRANT SELECT ON tenant.clusters TO fun_authz_worker;
GRANT SELECT ON tenant.node_pools TO fun_authz_worker;
GRANT SELECT ON tenant.namespaces TO fun_authz_worker;
GRANT SELECT ON tenant.organizations TO fun_authz_worker;

GRANT SELECT ON authn.api_keys TO fun_authz_worker;

GRANT SELECT ON zappstore.installs TO fun_authz_worker;
