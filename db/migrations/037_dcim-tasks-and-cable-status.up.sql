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

-- A task carries tags instead of one category.
--
-- "Switch R01-2 vervangen" is network work and hardware work at once, and a
-- single category forced a choice between them: whichever you picked, the other
-- half of the truth was gone. Tags also travel between systems, which a closed
-- category does not — the tasks in here are going to arrive from more than one
-- app.
--
-- The tags themselves are free text for now, keyed per task. A curated set (a
-- dcim.tags table with a foreign key) can be added later without touching the
-- API: what a client sends stays a list of strings either way.
--
-- Every task keeps what it had: its category becomes its first tag. 'other' is
-- dropped rather than carried over, because a tag that says "uncategorized"
-- says less than no tag at all.
--
-- Not reversible: a task with two tags has no single category to fold back into.

CREATE TABLE "dcim"."task_tags" (
	"task_id" uuid NOT NULL,
	"tag" text COLLATE "pg_catalog"."default" NOT NULL,
	"created" timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE "dcim"."task_tags" ADD CONSTRAINT "task_tags_pk" PRIMARY KEY ("task_id", "tag");

ALTER TABLE "dcim"."task_tags" ADD CONSTRAINT "task_tags_fk_task" FOREIGN KEY ("task_id") REFERENCES "dcim"."tasks"("id") ON DELETE CASCADE;

-- Reading "which tasks carry this tag" is what the menu does on every page load.
CREATE INDEX task_tags_ix_tag ON dcim.task_tags USING btree (tag);

INSERT INTO "dcim"."task_tags" ("task_id", "tag")
SELECT "id", "category" FROM "dcim"."tasks" WHERE "category" <> 'other';

ALTER TABLE "dcim"."tasks" DROP CONSTRAINT "tasks_ck_category";

ALTER TABLE "dcim"."tasks" DROP COLUMN "category";

GRANT INSERT ON "dcim"."task_tags" TO "fun_dcim_api";

GRANT SELECT ON "dcim"."task_tags" TO "fun_dcim_api";

GRANT DELETE ON "dcim"."task_tags" TO "fun_dcim_api";

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

-- Status says how far a cable is towards existing. Each value names what has to
-- happen next.
--
-- 'planned' named none of them. It only said the cable was not there yet, so
-- "have we bought this" had nowhere to live and the same list came up for
-- ordering week after week. It is renamed to 'to_order', and two states are
-- added behind it: 'ordered' for what is on its way, and 'ready_to_install' for
-- what has arrived and is waiting for hands. That last one was missing
-- altogether, which put a cable on a lorry and a cable lying in the rack in the
-- same state.
--
-- Every existing row moves from 'planned' to 'to_order'. That is the earliest of
-- the three and the only one that claims nothing on somebody's behalf: this
-- database never recorded whether anything had been bought, so it cannot say so
-- now.
--
-- A cable that is drawn but not decided on keeps no status at all (NULL), which
-- is what it already meant.
--
-- Not reversible: 'ordered' and 'ready_to_install' cannot be folded back into
-- 'planned' without losing which of the three a row was in.

ALTER TABLE "dcim"."physical_connections" DROP CONSTRAINT "physical_connections_ck_status";

UPDATE "dcim"."physical_connections" SET "status" = 'to_order' WHERE "status" = 'planned';

ALTER TABLE "dcim"."physical_connections" ADD CONSTRAINT "physical_connections_ck_status" CHECK(((status IS NULL) OR (status = ANY (ARRAY['to_order'::text, 'ordered'::text, 'ready_to_install'::text, 'connected'::text, 'decommissioned'::text]))));
