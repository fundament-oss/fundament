SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

ALTER TABLE "tenant"."clusters" ADD COLUMN "prometheus_url" text COLLATE "pg_catalog"."default" DEFAULT ''::text NOT NULL;
