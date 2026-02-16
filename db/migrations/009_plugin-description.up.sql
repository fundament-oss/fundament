SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

ALTER TABLE "appstore"."plugins" ADD COLUMN "description_short" text COLLATE "pg_catalog"."default" DEFAULT ''::text NOT NULL;

