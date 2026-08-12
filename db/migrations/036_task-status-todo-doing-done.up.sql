SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

-- Status says how far the work has got. Nothing else.
--
-- 'review' and 'blocked' were answering a different question: not how far the
-- work is, but whose turn it is. That is why "done" did not mean done — a task
-- could be finished on the floor and still sit in 'review'. And a blocked task
-- lost how far it had come, because 'blocked' overwrote that.
--
-- Whose turn it is now follows from the assignee: assigned to you means yours
-- to do, assigned to somebody else means you are waiting for them. What is left
-- is being stuck on something that is not a person — a part on order, access to
-- a room — and that gets a reason of its own.
--
-- Both old values map onto 'doing': the work was under way and had not been
-- finished. A row that was 'blocked' keeps that fact in blocked_reason, without
-- a reason, because the old model never recorded one.
--
-- Not reversible: 'doing' cannot be split back into review, blocked and in
-- progress.

ALTER TABLE "dcim"."tasks" ADD COLUMN "blocked_reason" text COLLATE "pg_catalog"."default";

UPDATE "dcim"."tasks" SET "blocked_reason" = '' WHERE "status" = 'blocked';

ALTER TABLE "dcim"."tasks" DROP CONSTRAINT "tasks_ck_status";

UPDATE "dcim"."tasks" SET "status" = 'todo'  WHERE "status" = 'ready';
UPDATE "dcim"."tasks" SET "status" = 'doing' WHERE "status" IN ('in_progress', 'review', 'blocked');

ALTER TABLE "dcim"."tasks" ALTER COLUMN "status" SET DEFAULT 'todo'::text;

ALTER TABLE "dcim"."tasks" ADD CONSTRAINT "tasks_ck_status" CHECK((status = ANY (ARRAY['todo'::text, 'doing'::text, 'done'::text])));
