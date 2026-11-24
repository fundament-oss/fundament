SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

-- Hand-added statements. trek's diff emits table-wide grants but not
-- schema-level or column-level ones, even when they are modelled in the .dbm,
-- so every statement marked "hand-added" in this file must be re-added whenever
-- the migration is regenerated.
--
-- The admin role resolves the reviewer of a submission to a name
-- (ListReviewers, FUN-20): it reads a staff member's id and name, nothing more.
GRANT USAGE ON SCHEMA "dcim" TO "fun_marketplace_admin_api";
GRANT SELECT ("id") ON "dcim"."users" TO "fun_marketplace_admin_api";
GRANT SELECT ("name") ON "dcim"."users" TO "fun_marketplace_admin_api";

-- Hand-added. The analysis worker reads the image of the version it analyzes
-- (FUN-24), and only the columns needed to find and check it.
GRANT USAGE ON SCHEMA "appstore" TO "fun_marketplace_analysis_worker";
GRANT SELECT ("id") ON "appstore"."plugin_definitions" TO "fun_marketplace_analysis_worker";
GRANT SELECT ("image") ON "appstore"."plugin_definitions" TO "fun_marketplace_analysis_worker";
GRANT SELECT ("status") ON "appstore"."plugin_definitions" TO "fun_marketplace_analysis_worker";
GRANT SELECT ("deleted") ON "appstore"."plugin_definitions" TO "fun_marketplace_analysis_worker";

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION appstore.plugin_image_analysis_enqueue()
 RETURNS trigger
 LANGUAGE plpgsql
 SECURITY DEFINER COST 1
 SET search_path TO 'pg_catalog'
AS $function$
BEGIN
    INSERT INTO appstore.plugin_image_analyses (plugin_definition_id)
    VALUES (NEW.plugin_definition_id)
    ON CONFLICT ON CONSTRAINT plugin_image_analyses_uq_plugin_definition_id DO UPDATE
    SET status = 'pending',
        status_info = NULL,
        retries = 0,
        retry_after = NULL
    WHERE appstore.plugin_image_analyses.status = 'failed';
    PERFORM pg_notify('plugin_image_analysis', '');
    RETURN NEW;
END;
$function$
;

/* Hazards:
 - AUTHZ_UPDATE: Adding a permissive policy could allow unauthorized access to data.
*/
CREATE POLICY "plugin_definitions_select_analysis_worker" ON "appstore"."plugin_definitions"
	AS PERMISSIVE
	FOR SELECT
	TO fun_marketplace_analysis_worker
	USING (true);

CREATE TABLE "appstore"."plugin_image_analyses" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"plugin_definition_id" uuid NOT NULL,
	"status" text COLLATE "pg_catalog"."default" DEFAULT 'pending'::text NOT NULL,
	"status_info" text COLLATE "pg_catalog"."default",
	"retries" integer DEFAULT 0 NOT NULL,
	"retry_after" timestamp with time zone,
	"image_digest" text COLLATE "pg_catalog"."default" DEFAULT ''::text NOT NULL,
	"platform" text COLLATE "pg_catalog"."default" DEFAULT ''::text NOT NULL,
	"platforms" jsonb DEFAULT '[]'::jsonb NOT NULL,
	"packages" jsonb DEFAULT '[]'::jsonb NOT NULL,
	"files" jsonb DEFAULT '[]'::jsonb NOT NULL,
	"files_truncated" boolean DEFAULT false NOT NULL,
	"sbom" bytea,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"analyzed" timestamp with time zone,
	"deleted" timestamp with time zone
);

ALTER TABLE "appstore"."plugin_image_analyses" ADD CONSTRAINT "plugin_image_analyses_ck_analyzed" CHECK(((status = 'completed'::text) = (analyzed IS NOT NULL)));

ALTER TABLE "appstore"."plugin_image_analyses" ADD CONSTRAINT "plugin_image_analyses_ck_status" CHECK((status = ANY (ARRAY['pending'::text, 'running'::text, 'retrying'::text, 'completed'::text, 'failed'::text])));

CREATE POLICY "plugin_image_analyses_all_analysis_worker" ON "appstore"."plugin_image_analyses"
	AS PERMISSIVE
	FOR ALL
	TO fun_marketplace_analysis_worker
	USING (true)
	WITH CHECK (true);

CREATE POLICY "plugin_image_analyses_select_admin" ON "appstore"."plugin_image_analyses"
	AS PERMISSIVE
	FOR SELECT
	TO fun_marketplace_admin_api
	USING (true);

ALTER TABLE "appstore"."plugin_image_analyses" ENABLE ROW LEVEL SECURITY;

GRANT SELECT ON "appstore"."plugin_image_analyses" TO "fun_marketplace_admin_api";

GRANT INSERT ON "appstore"."plugin_image_analyses" TO "fun_marketplace_analysis_worker";

GRANT SELECT ON "appstore"."plugin_image_analyses" TO "fun_marketplace_analysis_worker";

GRANT UPDATE ON "appstore"."plugin_image_analyses" TO "fun_marketplace_analysis_worker";

ALTER TABLE "appstore"."plugin_image_analyses" ADD CONSTRAINT "plugin_image_analyses_fk_plugin_definition" FOREIGN KEY (plugin_definition_id) REFERENCES appstore.plugin_definitions(id) NOT VALID;

ALTER TABLE "appstore"."plugin_image_analyses" VALIDATE CONSTRAINT "plugin_image_analyses_fk_plugin_definition";

CREATE UNIQUE INDEX plugin_image_analyses_pk ON appstore.plugin_image_analyses USING btree (id);

ALTER TABLE "appstore"."plugin_image_analyses" ADD CONSTRAINT "plugin_image_analyses_pk" PRIMARY KEY USING INDEX "plugin_image_analyses_pk";

CREATE UNIQUE INDEX plugin_image_analyses_uq_plugin_definition_id ON appstore.plugin_image_analyses USING btree (plugin_definition_id, deleted) NULLS NOT DISTINCT;

ALTER TABLE "appstore"."plugin_image_analyses" ADD CONSTRAINT "plugin_image_analyses_uq_plugin_definition_id" UNIQUE USING INDEX "plugin_image_analyses_uq_plugin_definition_id";

CREATE INDEX plugin_image_analyses_idx_claimable ON appstore.plugin_image_analyses USING btree (created) WHERE ((status = ANY (ARRAY['pending'::text, 'running'::text, 'retrying'::text])) AND (deleted IS NULL));

CREATE TRIGGER plugin_image_analysis_enqueue AFTER INSERT ON appstore.submissions FOR EACH ROW EXECUTE FUNCTION appstore.plugin_image_analysis_enqueue();


-- Statements generated automatically, please review:
ALTER FUNCTION appstore.plugin_image_analysis_enqueue() OWNER TO fun_owner;
ALTER TABLE appstore.plugin_image_analyses OWNER TO fun_owner;
