SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

ALTER TABLE "dcim"."notes" ADD COLUMN "created_by_id" uuid;

/* Hazards:
 - DELETES_DATA: Deletes all values in the column
*/
ALTER TABLE "dcim"."notes" DROP COLUMN "created_by";

ALTER TABLE "dcim"."notes" ADD CONSTRAINT "dcim_notes_fk_created_by" FOREIGN KEY (created_by_id) REFERENCES dcim.users(id) NOT VALID;

ALTER TABLE "dcim"."notes" VALIDATE CONSTRAINT "dcim_notes_fk_created_by";

