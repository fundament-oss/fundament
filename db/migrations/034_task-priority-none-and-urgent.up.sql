SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

-- Priority gains an empty state and loses the word "critical".
--
-- "None" is what a task carries until somebody prioritizes it. It used to land
-- on 'medium', so a task nobody had looked at was indistinguishable from one
-- deliberately set to medium. The default moves with it: a row that arrives
-- without a priority now says so.
--
-- "Critical" becomes "urgent" because a priority says when to act, while
-- critical describes the state of a thing — and a data center has plenty of
-- those already (a cluster, a certificate, a cooling loop). Same rank, other
-- word: every row keeps its meaning.
--
-- Not reversible in the strict sense. Going back would have to fold 'none' into
-- something, and 'medium' is the only place it could go — which is exactly the
-- confusion this migration removes.

ALTER TABLE "dcim"."tasks" DROP CONSTRAINT "tasks_ck_priority";

UPDATE "dcim"."tasks" SET "priority" = 'urgent' WHERE "priority" = 'critical';

ALTER TABLE "dcim"."tasks" ALTER COLUMN "priority" SET DEFAULT 'none'::text;

ALTER TABLE "dcim"."tasks" ADD CONSTRAINT "tasks_ck_priority" CHECK((priority = ANY (ARRAY['none'::text, 'low'::text, 'medium'::text, 'high'::text, 'urgent'::text])));
