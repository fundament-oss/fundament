SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

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
