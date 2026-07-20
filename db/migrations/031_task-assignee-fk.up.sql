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

ALTER TABLE "dcim"."tasks" ADD CONSTRAINT "dcim_tasks_fk_assignee" FOREIGN KEY (assignee_id) REFERENCES dcim.users(id) NOT VALID;

ALTER TABLE "dcim"."tasks" VALIDATE CONSTRAINT "dcim_tasks_fk_assignee";

