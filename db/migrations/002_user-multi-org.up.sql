SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;


ALTER INDEX "tenant"."users_uq_external_id" RENAME TO "pgschemadiff_tmpidx_users_uq_external_id_m2r7uRMXQ2etqu_zKDggLw";

CREATE TABLE "tenant"."organizations_users" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"organization_id" uuid NOT NULL,
	"user_id" uuid NOT NULL,
	"permission" text COLLATE "pg_catalog"."default" NOT NULL,
  "status" text COLLATE "pg_catalog"."default" NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

ALTER TABLE "tenant"."organizations_users" ADD CONSTRAINT "organizations_users_ck_permission" CHECK((permission = ANY (ARRAY['admin'::text, 'viewer'::text])));

ALTER TABLE "tenant"."organizations_users" ADD CONSTRAINT "organizations_users_ck_status" CHECK((status = ANY (ARRAY['pending'::text, 'accepted'::text, 'declined'::text, 'revoked'::text]))) NOT VALID;

ALTER TABLE "tenant"."organizations_users" VALIDATE CONSTRAINT "organizations_users_ck_status";

CREATE UNIQUE INDEX organizations_users_pk ON tenant.organizations_users USING btree (id);

ALTER TABLE "tenant"."organizations_users" ADD CONSTRAINT "organizations_users_pk" PRIMARY KEY USING INDEX "organizations_users_pk";

CREATE UNIQUE INDEX organizations_users_uq_user ON tenant.organizations_users USING btree (organization_id, user_id) WHERE ((deleted IS NULL) AND (status <> ALL (ARRAY['declined'::text, 'revoked'::text])));

ALTER TABLE "tenant"."users" DROP CONSTRAINT "users_fk_organization";

ALTER TABLE "tenant"."organizations_users" ADD CONSTRAINT "organizations_users_fk_organization" FOREIGN KEY (organization_id) REFERENCES tenant.organizations(id) NOT VALID;

ALTER TABLE "tenant"."organizations_users" VALIDATE CONSTRAINT "organizations_users_fk_organization";

ALTER TABLE "authz"."outbox" ADD COLUMN "organization_user_id" uuid;

ALTER TABLE "authz"."outbox" DROP CONSTRAINT "outbox_ck_single_fk";

DELETE FROM "authz"."outbox" WHERE user_id IS NOT NULL;

ALTER TABLE "authz"."outbox" DROP CONSTRAINT "outbox_fk_user";

ALTER TABLE "authz"."outbox" DROP COLUMN "user_id";

ALTER TABLE "authz"."outbox" ADD CONSTRAINT "outbox_ck_single_fk" CHECK((num_nonnulls(project_id, project_member_id, cluster_id, node_pool_id, namespace_id, api_key_id, install_id, organization_user_id) = 1)) NOT VALID;

ALTER TABLE "authz"."outbox" VALIDATE CONSTRAINT "outbox_ck_single_fk";

DROP TRIGGER "users_outbox" ON "tenant"."users";

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For drops, this means you need to ensure that all functions this function depends on are dropped after this statement.
*/
DROP FUNCTION "authz"."users_sync_trigger"();

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."organizations_users" TO "fun_authz_worker";

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.organizations_users_sync_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    -- Only insert into outbox if this is an INSERT or if data actually changed
    IF TG_OP = 'INSERT' OR NEW IS DISTINCT FROM OLD THEN
        INSERT INTO authz.outbox (organization_user_id)
        VALUES (COALESCE(NEW.id, OLD.id));
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

ALTER FUNCTION authz.organizations_users_sync_trigger() OWNER TO fun_owner;

CREATE TRIGGER organizations_users_outbox AFTER INSERT OR UPDATE ON tenant.organizations_users FOR EACH ROW EXECUTE FUNCTION authz.organizations_users_sync_trigger();

ALTER TABLE "authz"."outbox" ADD CONSTRAINT "outbox_fk_organization_user" FOREIGN KEY (organization_user_id) REFERENCES tenant.organizations_users(id) NOT VALID;

ALTER TABLE "authz"."outbox" VALIDATE CONSTRAINT "outbox_fk_organization_user";

INSERT INTO "tenant"."organizations_users" (organization_id, user_id, permission, status)
SELECT organization_id, id, role, 'accepted' FROM "tenant"."users";

/* Hazards:
 - ACQUIRES_ACCESS_EXCLUSIVE_LOCK: Index drops will lock out all accesses to the table. They should be fast.
 - INDEX_DROPPED: Dropping this index means queries that use this index might perform worse because they will no longer will be able to leverage it.
*/
ALTER TABLE "tenant"."users" DROP CONSTRAINT "pgschemadiff_tmpidx_users_uq_external_id_m2r7uRMXQ2etqu_zKDggLw";

ALTER TABLE "tenant"."users" ADD COLUMN "external_ref" text COLLATE "pg_catalog"."default";

UPDATE "tenant"."users" SET "external_ref" = "external_id";
/* Hazards:
 - DELETES_DATA: Deletes all values in the column
*/
ALTER TABLE "tenant"."users" DROP COLUMN "external_id";

/* Hazards:
 - DELETES_DATA: Deletes all values in the column
*/
ALTER TABLE "tenant"."users" DROP COLUMN "organization_id";

/* Hazards:
 - DELETES_DATA: Deletes all values in the column
*/
ALTER TABLE "tenant"."users" DROP COLUMN "role";

/* Hazards:
 - ACQUIRES_SHARE_LOCK: Non-concurrent index creates will lock out writes to the table during the duration of the index build.
*/
CREATE UNIQUE INDEX users_uq_external_ref ON tenant.users USING btree (external_ref) WHERE (deleted IS NULL);

ALTER TABLE "tenant"."organizations_users" ADD CONSTRAINT "organizations_users_fk_user" FOREIGN KEY (user_id) REFERENCES tenant.users(id) NOT VALID;

ALTER TABLE "tenant"."organizations_users" VALIDATE CONSTRAINT "organizations_users_fk_user";


-- Statements generated automatically, please review:
ALTER TABLE tenant.organizations_users OWNER TO fun_owner;

CREATE OR REPLACE FUNCTION authn.is_organization_member(p_organization_id uuid)
 RETURNS boolean
 LANGUAGE sql
 STABLE PARALLEL SAFE SECURITY DEFINER COST 1
AS $function$
SELECT EXISTS (
    SELECT 1 FROM tenant.organizations_users
    WHERE organization_id = p_organization_id
    AND user_id = authn.current_user_id()
    AND deleted IS NULL
)
$function$
;

CREATE POLICY "organizations_select_policy" ON "tenant"."organizations"
        AS PERMISSIVE
        FOR SELECT
        TO fun_fundament_api
        USING (authn.is_organization_member(id));

CREATE POLICY "organizations_authn_api_policy" ON "tenant"."organizations"
        AS PERMISSIVE
        FOR ALL
        TO fun_authn_api
        USING (true);

ALTER TABLE "tenant"."organizations" ENABLE ROW LEVEL SECURITY;

ALTER FUNCTION authn.is_organization_member(p_organization_id uuid) OWNER TO fun_authz;


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
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "users_select_policy" ON "tenant"."users"
        AS PERMISSIVE
        FOR SELECT
        TO fun_fundament_api
        USING (true);

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "users_insert_policy" ON "tenant"."users"
        AS PERMISSIVE
        FOR INSERT
        TO fun_fundament_api
        WITH CHECK (true);

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "users_authn_api_policy" ON "tenant"."users"
        AS PERMISSIVE
        FOR ALL
        TO fun_authn_api
        USING (true);

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
CREATE POLICY "organizations_users_insert_policy" ON "tenant"."organizations_users"
        AS PERMISSIVE
        FOR INSERT
        TO fun_fundament_api
        WITH CHECK ((organization_id = authn.current_organization_id()));

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
CREATE POLICY "organizations_users_authn_api_policy" ON "tenant"."organizations_users"
        AS PERMISSIVE
        FOR ALL
        TO fun_authn_api
        USING (true);

CREATE OR REPLACE FUNCTION authn.current_user_id()
 RETURNS uuid
 LANGUAGE sql
 STABLE PARALLEL SAFE COST 1
AS $function$
SELECT NULLIF(current_setting('app.current_user_id', true), '')::uuid
$function$
;

CREATE OR REPLACE FUNCTION authn.current_organization_id()
 RETURNS uuid
 LANGUAGE sql
 STABLE PARALLEL SAFE SECURITY INVOKER COST 1
AS $function$
SELECT NULLIF(current_setting('app.current_organization_id', true), '')::uuid
$function$
;

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "organizations_users_user_select_policy" ON "tenant"."organizations_users"
        AS PERMISSIVE
        FOR SELECT
        TO fun_fundament_api
        USING ((user_id = authn.current_user_id()));

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "organizations_users_user_update_policy" ON "tenant"."organizations_users"
        AS PERMISSIVE
        FOR UPDATE
        TO fun_fundament_api
        USING ((user_id = authn.current_user_id()));


/* Hazards:
 - AUTHZ_UPDATE: Enabling RLS on a table could cause queries to fail if not correctly configured.
*/
ALTER TABLE "tenant"."organizations_users" ENABLE ROW LEVEL SECURITY;


/* Hazards:
 - AUTHZ_UPDATE: Enabling RLS on a table could cause queries to fail if not correctly configured.
*/
ALTER TABLE "tenant"."users" ENABLE ROW LEVEL SECURITY;

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."organizations_users" TO "fun_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "tenant"."organizations_users" TO "fun_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "tenant"."organizations_users" TO "fun_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."organizations_users" TO "fun_authz";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "tenant"."organizations_users" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."organizations_users" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "tenant"."organizations_users" TO "fun_fundament_api";

