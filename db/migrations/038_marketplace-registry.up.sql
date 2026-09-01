SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

-- Hand-added statements. trek's diff emits table grants but not schema-level or
-- column-level permissions, even when they are modelled in the .dbm, so
-- everything in this block must be re-added whenever this migration is
-- regenerated.
--
-- Schema USAGE for the registry role:
--   appstore: without it every table grant below is unusable.
--   authn: the role's own policies call authn.current_organization_id().
--   authz: appstore.plugins carries the plugins_outbox trigger, which runs
--     authz.plugins_sync_trigger() SECURITY INVOKER to queue an OpenFGA sync. Any
--     role that writes a plugin therefore writes authz.outbox as itself, so
--     without this every INSERT and UPDATE fails with "permission denied for
--     schema authz".
GRANT USAGE ON SCHEMA "appstore" TO "fun_marketplace_registry_api";
GRANT USAGE ON SCHEMA "authn" TO "fun_marketplace_registry_api";
GRANT USAGE ON SCHEMA "authz" TO "fun_marketplace_registry_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."categories" TO "fun_marketplace_registry_api";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "categories_plugins_all_registry" ON "appstore"."categories_plugins"
	AS PERMISSIVE
	FOR ALL
	TO fun_marketplace_registry_api
	USING ((EXISTS ( SELECT 1
   FROM appstore.plugins
  WHERE ((plugins.id = categories_plugins.plugin_id) AND (plugins.organization_id = authn.current_organization_id())))))
	WITH CHECK ((EXISTS ( SELECT 1
   FROM appstore.plugins
  WHERE ((plugins.id = categories_plugins.plugin_id) AND (plugins.organization_id = authn.current_organization_id())))));

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT DELETE ON "appstore"."categories_plugins" TO "fun_marketplace_registry_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "appstore"."categories_plugins" TO "fun_marketplace_registry_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."categories_plugins" TO "fun_marketplace_registry_api";

CREATE TABLE "appstore"."plugin_allowed_organizations" (
	"plugin_id" uuid NOT NULL,
	"organization_id" uuid NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL
);

CREATE POLICY "plugin_allowed_organizations_all_registry" ON "appstore"."plugin_allowed_organizations"
	AS PERMISSIVE
	FOR ALL
	TO fun_marketplace_registry_api
	USING ((EXISTS ( SELECT 1
   FROM appstore.plugins
  WHERE ((plugins.id = plugin_allowed_organizations.plugin_id) AND (plugins.organization_id = authn.current_organization_id())))))
	WITH CHECK ((EXISTS ( SELECT 1
   FROM appstore.plugins
  WHERE ((plugins.id = plugin_allowed_organizations.plugin_id) AND (plugins.organization_id = authn.current_organization_id())))));

CREATE POLICY "plugin_allowed_organizations_select_api" ON "appstore"."plugin_allowed_organizations"
	AS PERMISSIVE
	FOR SELECT
	TO fun_fundament_api
	USING ((organization_id = authn.current_organization_id()));

ALTER TABLE "appstore"."plugin_allowed_organizations" ENABLE ROW LEVEL SECURITY;

GRANT SELECT ON "appstore"."plugin_allowed_organizations" TO "fun_fundament_api";

GRANT DELETE ON "appstore"."plugin_allowed_organizations" TO "fun_marketplace_registry_api";

GRANT INSERT ON "appstore"."plugin_allowed_organizations" TO "fun_marketplace_registry_api";

GRANT SELECT ON "appstore"."plugin_allowed_organizations" TO "fun_marketplace_registry_api";

CREATE UNIQUE INDEX plugin_allowed_organizations_pk ON appstore.plugin_allowed_organizations USING btree (plugin_id, organization_id);

ALTER TABLE "appstore"."plugin_allowed_organizations" ADD CONSTRAINT "plugin_allowed_organizations_pk" PRIMARY KEY USING INDEX "plugin_allowed_organizations_pk";

CREATE INDEX plugin_allowed_organizations_idx_organization_id ON appstore.plugin_allowed_organizations USING btree (organization_id);

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "plugin_definitions_all_registry" ON "appstore"."plugin_definitions"
	AS PERMISSIVE
	FOR ALL
	TO fun_marketplace_registry_api
	USING ((EXISTS ( SELECT 1
   FROM appstore.plugins
  WHERE ((plugins.id = plugin_definitions.plugin_id) AND (plugins.organization_id = authn.current_organization_id())))))
	WITH CHECK ((EXISTS ( SELECT 1
   FROM appstore.plugins
  WHERE ((plugins.id = plugin_definitions.plugin_id) AND (plugins.organization_id = authn.current_organization_id())))));

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "appstore"."plugin_definitions" TO "fun_marketplace_registry_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."plugin_definitions" TO "fun_marketplace_registry_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "appstore"."plugin_definitions" TO "fun_marketplace_registry_api";

ALTER TABLE "appstore"."plugin_documentation_links" ADD COLUMN "deleted" timestamp with time zone;

ALTER TABLE "appstore"."plugin_documentation_links" ADD COLUMN "position" integer DEFAULT 0 NOT NULL;

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "plugin_documentation_links_all_registry" ON "appstore"."plugin_documentation_links"
	AS PERMISSIVE
	FOR ALL
	TO fun_marketplace_registry_api
	USING ((EXISTS ( SELECT 1
   FROM appstore.plugins
  WHERE ((plugins.id = plugin_documentation_links.plugin_id) AND (plugins.organization_id = authn.current_organization_id())))))
	WITH CHECK ((EXISTS ( SELECT 1
   FROM appstore.plugins
  WHERE ((plugins.id = plugin_documentation_links.plugin_id) AND (plugins.organization_id = authn.current_organization_id())))));

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "appstore"."plugin_documentation_links" TO "fun_marketplace_registry_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."plugin_documentation_links" TO "fun_marketplace_registry_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "appstore"."plugin_documentation_links" TO "fun_marketplace_registry_api";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "plugin_features_all_registry" ON "appstore"."plugin_features"
	AS PERMISSIVE
	FOR ALL
	TO fun_marketplace_registry_api
	USING ((EXISTS ( SELECT 1
   FROM appstore.plugins
  WHERE ((plugins.id = plugin_features.plugin_id) AND (plugins.organization_id = authn.current_organization_id())))))
	WITH CHECK ((EXISTS ( SELECT 1
   FROM appstore.plugins
  WHERE ((plugins.id = plugin_features.plugin_id) AND (plugins.organization_id = authn.current_organization_id())))));

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "appstore"."plugin_features" TO "fun_marketplace_registry_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."plugin_features" TO "fun_marketplace_registry_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "appstore"."plugin_features" TO "fun_marketplace_registry_api";

ALTER TABLE "appstore"."plugins" ADD COLUMN "updated" timestamp with time zone DEFAULT now() NOT NULL;

-- Hand-added: the column default stamps every pre-existing row with the
-- migration timestamp, so registry.v1.Plugin.updated would report the whole
-- catalog as just-updated at deploy. Nothing has been updated since it was
-- created, so say that. Re-add whenever this migration is regenerated.
UPDATE appstore.plugins SET updated = created;

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "plugins_all_registry" ON "appstore"."plugins"
	AS PERMISSIVE
	FOR ALL
	TO fun_marketplace_registry_api
	USING ((organization_id = authn.current_organization_id()))
	WITH CHECK ((organization_id = authn.current_organization_id()));

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "appstore"."plugins" TO "fun_marketplace_registry_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."plugins" TO "fun_marketplace_registry_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "appstore"."plugins" TO "fun_marketplace_registry_api";

ALTER TABLE "appstore"."plugin_allowed_organizations" ADD CONSTRAINT "plugin_allowed_organizations_fk_plugin" FOREIGN KEY (plugin_id) REFERENCES appstore.plugins(id) NOT VALID;

ALTER TABLE "appstore"."plugin_allowed_organizations" VALIDATE CONSTRAINT "plugin_allowed_organizations_fk_plugin";

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "plugins_tags_all_registry" ON "appstore"."plugins_tags"
	AS PERMISSIVE
	FOR ALL
	TO fun_marketplace_registry_api
	USING ((EXISTS ( SELECT 1
   FROM appstore.plugins
  WHERE ((plugins.id = plugins_tags.plugin_id) AND (plugins.organization_id = authn.current_organization_id())))))
	WITH CHECK ((EXISTS ( SELECT 1
   FROM appstore.plugins
  WHERE ((plugins.id = plugins_tags.plugin_id) AND (plugins.organization_id = authn.current_organization_id())))));

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT DELETE ON "appstore"."plugins_tags" TO "fun_marketplace_registry_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "appstore"."plugins_tags" TO "fun_marketplace_registry_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."plugins_tags" TO "fun_marketplace_registry_api";

CREATE TABLE "appstore"."submissions" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"plugin_definition_id" uuid NOT NULL,
	"submitter_user_id" uuid NOT NULL,
	"reviewer_user_id" uuid,
	"rejection_reason" text COLLATE "pg_catalog"."default",
	"feedback" text COLLATE "pg_catalog"."default" DEFAULT ''::text NOT NULL,
	"submitted" timestamp with time zone DEFAULT now() NOT NULL,
	"reviewed" timestamp with time zone,
	"closed" timestamp with time zone,
	"deleted" timestamp with time zone
);

ALTER TABLE "appstore"."submissions" ADD CONSTRAINT "submissions_ck_closed" CHECK(((reviewed IS NULL) OR (closed IS NOT NULL)));

ALTER TABLE "appstore"."submissions" ADD CONSTRAINT "submissions_ck_rejection_reason" CHECK(((rejection_reason IS NULL) OR (rejection_reason = ANY (ARRAY['incomplete_metadata'::text, 'duplicate'::text, 'security_concerns'::text, 'naming_guidelines'::text, 'out_of_scope'::text, 'other'::text]))));

ALTER TABLE "appstore"."submissions" ADD CONSTRAINT "submissions_ck_reviewed" CHECK(((reviewed IS NULL) = (reviewer_user_id IS NULL)));

CREATE POLICY "submissions_all_registry" ON "appstore"."submissions"
	AS PERMISSIVE
	FOR ALL
	TO fun_marketplace_registry_api
	USING ((EXISTS ( SELECT 1
   FROM (appstore.plugin_definitions
     JOIN appstore.plugins ON ((plugins.id = plugin_definitions.plugin_id)))
  WHERE ((plugin_definitions.id = submissions.plugin_definition_id) AND (plugins.organization_id = authn.current_organization_id())))))
	WITH CHECK ((EXISTS ( SELECT 1
   FROM (appstore.plugin_definitions
     JOIN appstore.plugins ON ((plugins.id = plugin_definitions.plugin_id)))
  WHERE ((plugin_definitions.id = submissions.plugin_definition_id) AND (plugins.organization_id = authn.current_organization_id())))));

ALTER TABLE "appstore"."submissions" ENABLE ROW LEVEL SECURITY;

GRANT INSERT ON "appstore"."submissions" TO "fun_marketplace_registry_api";

GRANT SELECT ON "appstore"."submissions" TO "fun_marketplace_registry_api";

-- Column-scoped so WithdrawPluginVersion can close a round and a plugin delete
-- can soft-delete one, while reviewed, reviewer_user_id, rejection_reason and
-- feedback stay out of the publisher's reach.
GRANT UPDATE ("closed") ON "appstore"."submissions" TO "fun_marketplace_registry_api";
GRANT UPDATE ("deleted") ON "appstore"."submissions" TO "fun_marketplace_registry_api";

ALTER TABLE "appstore"."submissions" ADD CONSTRAINT "submissions_fk_plugin_definition" FOREIGN KEY (plugin_definition_id) REFERENCES appstore.plugin_definitions(id) NOT VALID;

ALTER TABLE "appstore"."submissions" VALIDATE CONSTRAINT "submissions_fk_plugin_definition";

CREATE UNIQUE INDEX submissions_pk ON appstore.submissions USING btree (id);

ALTER TABLE "appstore"."submissions" ADD CONSTRAINT "submissions_pk" PRIMARY KEY USING INDEX "submissions_pk";

CREATE UNIQUE INDEX submissions_uq_open ON appstore.submissions USING btree (plugin_definition_id) WHERE ((closed IS NULL) AND (deleted IS NULL));

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "appstore"."tags" TO "fun_marketplace_registry_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "appstore"."tags" TO "fun_marketplace_registry_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "authz"."outbox" TO "fun_marketplace_registry_api";

ALTER TABLE "appstore"."plugin_allowed_organizations" ADD CONSTRAINT "plugin_allowed_organizations_fk_organization" FOREIGN KEY (organization_id) REFERENCES tenant.organizations(id) NOT VALID;

ALTER TABLE "appstore"."plugin_allowed_organizations" VALIDATE CONSTRAINT "plugin_allowed_organizations_fk_organization";


-- Statements generated automatically, please review:
ALTER TABLE appstore.plugin_allowed_organizations OWNER TO fun_owner;
ALTER TABLE appstore.submissions OWNER TO fun_owner;
