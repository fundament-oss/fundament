-- Introduces the DCIM user directory and points task assignees and note authors
-- at it.
--
-- NOT DATA-SAFE ON A POPULATED DATABASE. This migration assumes dcim.tasks holds
-- no assignees and dcim.notes no authorship worth keeping:
--   * the assignee_id cast below fails outright on any value that is not a uuid;
--   * a well-formed one then fails the foreign key, because dcim.users is
--     created empty here and nothing has been provisioned into it;
--   * dropping notes.created_by loses the existing free-text authorship
--     outright — there is no backfill, because there is nothing to map those
--     names onto until dcim.users is provisioned. That is a deliberate exception
--     to the repo-wide soft-delete rule: the column goes, and with it the data.
-- That holds at the time of writing — no environment carries real task or note
-- data. Before this reaches one that does, provision dcim.users first and rework
-- this migration to backfill assignee_id and created_by_id from it rather than
-- assuming an empty start. db/testdata/030_0101-content.sql does exactly that
-- mapping for the local seed data.

SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

CREATE TABLE "dcim"."users" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"external_ref" text COLLATE "pg_catalog"."default",
	"name" text COLLATE "pg_catalog"."default" NOT NULL,
	"email" text COLLATE "pg_catalog"."default",
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

GRANT SELECT ON "dcim"."users" TO "fun_dcim_api";

CREATE UNIQUE INDEX users_pk ON dcim.users USING btree (id);

ALTER TABLE "dcim"."users" ADD CONSTRAINT "users_pk" PRIMARY KEY USING INDEX "users_pk";

-- Prefixed with the schema name because tenant.users already carries a
-- users_uq_external_ref. Postgres scopes index names per schema, so both can
-- coexist — but common/dbconst is generated as one flat namespace keyed on the
-- bare constraint name, so two same-named constraints collapse onto a single Go
-- constant and error mapping can no longer tell the two tables apart.
CREATE UNIQUE INDEX dcim_users_uq_external_ref ON dcim.users USING btree (external_ref) WHERE (deleted IS NULL);

/* Hazards:
 - ACQUIRES_ACCESS_EXCLUSIVE_LOCK: This will completely lock the table while the data is being re-written. The duration of this conversion depends on if the type conversion is trivial or not. A non-trivial conversion will require a table rewrite. A trivial conversion is one where the binary values are coercible and the column contents are not changing.
*/
ALTER TABLE "dcim"."tasks" ALTER COLUMN "assignee_id" SET DATA TYPE uuid using "assignee_id"::uuid;

SET SESSION statement_timeout = 1200000;

/* Hazards:
 - IMPACTS_DATABASE_PERFORMANCE: Running analyze will read rows from the table, putting increased load on the database and consuming database resources. It won't prevent reads/writes to the table, but it could affect performance when executing queries.
*/
ANALYZE "dcim"."tasks" ("assignee_id");

SET SESSION statement_timeout = 3000;

-- Added validated in one statement, rather than NOT VALID followed by an
-- immediate VALIDATE. Splitting the two only shortens the lock window when the
-- VALIDATE runs in its own transaction; back to back in one migration the locks
-- are held together anyway. And the type change above already took ACCESS
-- EXCLUSIVE and rewrote the table, so there is no window left to protect.
ALTER TABLE "dcim"."tasks" ADD CONSTRAINT "dcim_tasks_fk_assignee" FOREIGN KEY (assignee_id) REFERENCES dcim.users(id);

ALTER TABLE "dcim"."notes" ADD COLUMN "created_by_id" uuid;

/* Hazards:
 - DELETES_DATA: Deletes all values in the column
*/
ALTER TABLE "dcim"."notes" DROP COLUMN "created_by";

-- Added validated in one statement for the same reason as the assignee foreign
-- key above: created_by_id was added in this same migration and is therefore
-- NULL on every row, so the validation scan has nothing to reject.
ALTER TABLE "dcim"."notes" ADD CONSTRAINT "dcim_notes_fk_created_by" FOREIGN KEY (created_by_id) REFERENCES dcim.users(id);


-- Statements generated automatically, please review:
ALTER TABLE dcim.users OWNER TO fun_owner;
