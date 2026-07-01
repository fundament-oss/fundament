SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

CREATE TABLE "dcim"."users" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"name" text COLLATE "pg_catalog"."default" NOT NULL,
	"email" text COLLATE "pg_catalog"."default",
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

GRANT SELECT ON "dcim"."users" TO "fun_dcim_api";

CREATE UNIQUE INDEX users_pk ON dcim.users USING btree (id);

ALTER TABLE "dcim"."users" ADD CONSTRAINT "users_pk" PRIMARY KEY USING INDEX "users_pk";


-- Statements generated automatically, please review:
ALTER TABLE dcim.users OWNER TO fun_owner;
