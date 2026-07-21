-- NOT DATA-SAFE ON A POPULATED DATABASE. Dropping created_by loses the existing
-- free-text authorship outright: there is no backfill, because there is nothing
-- to map those names onto until dcim.users is provisioned. Note that this is a
-- deliberate exception to the repo-wide soft-delete rule — the column goes, and
-- with it the data.
-- That is acceptable only while no environment carries real note data, which
-- holds at the time of writing. Before this reaches one that does, provision
-- dcim.users first and rework this migration to resolve created_by onto it
-- before the column is dropped. See also 031, which makes the same assumption
-- about task assignees. db/testdata/032_0101-content.sql does exactly this
-- mapping for the local seed data.

SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

ALTER TABLE "dcim"."notes" ADD COLUMN "created_by_id" uuid;

/* Hazards:
 - DELETES_DATA: Deletes all values in the column
*/
ALTER TABLE "dcim"."notes" DROP COLUMN "created_by";

-- Added validated in one statement rather than NOT VALID + an immediate
-- VALIDATE: created_by_id was added in this same migration and is therefore
-- NULL on every row, so the validation scan has nothing to reject and the split
-- buys no lock time. See the same note in 031.
ALTER TABLE "dcim"."notes" ADD CONSTRAINT "dcim_notes_fk_created_by" FOREIGN KEY (created_by_id) REFERENCES dcim.users(id);

