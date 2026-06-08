SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

ALTER TABLE "tenant"."projects" ADD COLUMN "alias" text COLLATE "pg_catalog"."default";

UPDATE "tenant"."projects" SET "alias" = "name";

ALTER TABLE "tenant"."projects" ALTER COLUMN "alias" SET NOT NULL;

ALTER TABLE "tenant"."projects" ADD CONSTRAINT "projects_ck_alias" CHECK(((char_length(alias) >= 1) AND (char_length(alias) <= 255))) NOT VALID;

ALTER TABLE "tenant"."projects" VALIDATE CONSTRAINT "projects_ck_alias";
