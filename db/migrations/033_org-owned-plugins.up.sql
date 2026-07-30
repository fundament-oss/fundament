SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

-- System organization owns first-party (seeded) plugins. Idempotent; the same
-- fixed UUID is (re)asserted by db/seed/0100-system-org.sql for fresh installs.
INSERT INTO "tenant"."organizations" ("id", "name", "alias")
VALUES ('019b4000-1000-7000-8000-000000000001', 'system', 'System')
ON CONFLICT ("id") DO NOTHING;

-- Add the owner column nullable first so existing rows can be backfilled.
ALTER TABLE "appstore"."plugins" ADD COLUMN "organization_id" uuid;

COMMENT ON COLUMN "appstore"."plugins"."organization_id" IS E'Owning organization. Gates who may publish/edit this plugin (RLS). Catalog visibility is NOT org-scoped — reads stay global.';

-- Backfill existing plugins to the system organization.
UPDATE "appstore"."plugins"
SET "organization_id" = '019b4000-1000-7000-8000-000000000001'
WHERE "organization_id" IS NULL;

-- Enforce NOT NULL and the FK now that every row has an owner.
ALTER TABLE "appstore"."plugins" ALTER COLUMN "organization_id" SET NOT NULL;

ALTER TABLE "appstore"."plugins" ADD CONSTRAINT "plugins_fk_organization" FOREIGN KEY ("organization_id") REFERENCES "tenant"."organizations"("id") MATCH SIMPLE ON DELETE NO ACTION ON UPDATE NO ACTION;

-- RLS on appstore.plugins: catalog reads stay GLOBAL (SELECT USING (true));
-- writes are gated to the owning organization. The UPDATE WITH CHECK also
-- prevents reassigning a plugin to another org (no ownership transfer).
CREATE POLICY "plugins_select_all" ON "appstore"."plugins"
	AS PERMISSIVE
	FOR SELECT
	TO fun_fundament_api
	USING (true);

CREATE POLICY "plugins_insert_owner" ON "appstore"."plugins"
	AS PERMISSIVE
	FOR INSERT
	TO fun_fundament_api
	WITH CHECK ((organization_id = authn.current_organization_id()));

CREATE POLICY "plugins_update_owner" ON "appstore"."plugins"
	AS PERMISSIVE
	FOR UPDATE
	TO fun_fundament_api
	USING ((organization_id = authn.current_organization_id()))
	WITH CHECK ((organization_id = authn.current_organization_id()));

CREATE POLICY "plugins_delete_owner" ON "appstore"."plugins"
	AS PERMISSIVE
	FOR DELETE
	TO fun_fundament_api
	USING ((organization_id = authn.current_organization_id()));

ALTER TABLE "appstore"."plugins" ENABLE ROW LEVEL SECURITY;

-- authz-worker reads appstore.plugins (GetPluginByID) to sync ownership to
-- OpenFGA. It already has USAGE on the appstore schema; grant table SELECT too.
-- Row visibility relies on fun_authz_worker's BYPASSRLS attribute (same as every
-- other RLS table it reads), so no per-worker policy is needed.
GRANT SELECT ON TABLE "appstore"."plugins" TO fun_authz_worker;

-- RLS on appstore.plugin_definitions: ownership lives on the parent plugins
-- row, so writes are gated by the parent's organization_id (no aliases).
CREATE POLICY "plugin_definitions_select_all" ON "appstore"."plugin_definitions"
	AS PERMISSIVE
	FOR SELECT
	TO fun_fundament_api
	USING (true);

CREATE POLICY "plugin_definitions_insert_owner" ON "appstore"."plugin_definitions"
	AS PERMISSIVE
	FOR INSERT
	TO fun_fundament_api
	WITH CHECK ((EXISTS (SELECT 1 FROM appstore.plugins WHERE appstore.plugins.id = appstore.plugin_definitions.plugin_id AND appstore.plugins.organization_id = authn.current_organization_id())));

CREATE POLICY "plugin_definitions_update_owner" ON "appstore"."plugin_definitions"
	AS PERMISSIVE
	FOR UPDATE
	TO fun_fundament_api
	USING ((EXISTS (SELECT 1 FROM appstore.plugins WHERE appstore.plugins.id = appstore.plugin_definitions.plugin_id AND appstore.plugins.organization_id = authn.current_organization_id())));

CREATE POLICY "plugin_definitions_delete_owner" ON "appstore"."plugin_definitions"
	AS PERMISSIVE
	FOR DELETE
	TO fun_fundament_api
	USING ((EXISTS (SELECT 1 FROM appstore.plugins WHERE appstore.plugins.id = appstore.plugin_definitions.plugin_id AND appstore.plugins.organization_id = authn.current_organization_id())));

ALTER TABLE "appstore"."plugin_definitions" ENABLE ROW LEVEL SECURITY;

-- OpenFGA authorization: sync plugin ownership (plugin owner organization) to
-- OpenFGA via the authz.outbox pipeline. A trigger on appstore.plugins enqueues
-- an outbox row on insert/update; the authz-worker writes the tuple. Because it
-- is a trigger, the seeded catalog plugins enqueue automatically.
ALTER TABLE "authz"."outbox" ADD COLUMN "plugin_id" uuid;

ALTER TABLE "authz"."outbox" DROP CONSTRAINT "outbox_ck_single_fk";

ALTER TABLE "authz"."outbox" ADD CONSTRAINT "outbox_ck_single_fk" CHECK (num_nonnulls(
	project_id,
	project_member_id,
	cluster_id,
	node_pool_id,
	namespace_id,
	api_key_id,
	organization_user_id,
	plugin_id
) = 1);

CREATE OR REPLACE FUNCTION authz.plugins_sync_trigger ()
	RETURNS trigger
	LANGUAGE plpgsql
	VOLATILE
	CALLED ON NULL INPUT
	SECURITY INVOKER
	PARALLEL UNSAFE
	COST 1
	AS
$function$
BEGIN
    -- Only insert into outbox if this is an INSERT, DELETE, or if data actually changed
    IF TG_OP = 'INSERT' OR NEW IS DISTINCT FROM OLD THEN
        INSERT INTO authz.outbox (plugin_id)
        VALUES (COALESCE(NEW.id, OLD.id));
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$function$;

ALTER FUNCTION authz.plugins_sync_trigger() OWNER TO fun_owner;

CREATE OR REPLACE TRIGGER plugins_outbox
	AFTER INSERT OR UPDATE
	ON appstore.plugins
	FOR EACH ROW
	EXECUTE PROCEDURE authz.plugins_sync_trigger();
