SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

ALTER TABLE "appstore"."plugin_definitions" ADD COLUMN "logo" text COLLATE "pg_catalog"."default" DEFAULT ''::text NOT NULL;

ALTER TABLE "appstore"."plugin_definitions" ADD COLUMN "logo_dark" text COLLATE "pg_catalog"."default" DEFAULT ''::text NOT NULL;

ALTER TABLE "appstore"."plugin_definitions" ADD COLUMN "menu_icons" jsonb DEFAULT '{}'::jsonb NOT NULL;

