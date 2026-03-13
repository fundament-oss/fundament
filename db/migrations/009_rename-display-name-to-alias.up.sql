SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

ALTER TABLE "tenant"."organizations" ADD COLUMN "alias" text COLLATE "pg_catalog"."default";

UPDATE "tenant"."organizations" SET "alias" = "display_name";

ALTER TABLE "tenant"."organizations" ALTER COLUMN "alias" SET NOT NULL;

ALTER TABLE "tenant"."organizations" ADD CONSTRAINT "organizations_ck_alias" CHECK(((char_length(alias) >= 1) AND (char_length(alias) <= 255))) NOT VALID;

ALTER TABLE "tenant"."organizations" VALIDATE CONSTRAINT "organizations_ck_alias";

ALTER TABLE "tenant"."organizations" DROP CONSTRAINT "organizations_ck_display_name";

/* Hazards:
 - DELETES_DATA: Deletes all values in the column
*/
ALTER TABLE "tenant"."organizations" DROP COLUMN "display_name";
