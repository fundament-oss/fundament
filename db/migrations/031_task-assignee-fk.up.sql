-- NOT DATA-SAFE ON A POPULATED DATABASE. This migration assumes dcim.tasks
-- holds no assignees yet:
--   * the cast below fails outright on any assignee_id that is not a uuid;
--   * a well-formed one then fails the foreign key, because dcim.users was
--     created empty one migration ago and nothing has been provisioned into it.
-- That holds at the time of writing — no environment carries real task data.
-- Before this reaches one that does, provision dcim.users first and rework this
-- migration to backfill assignee_id from it rather than assuming an empty start.
-- See also 032, which drops note authorship on the same assumption.

SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

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

