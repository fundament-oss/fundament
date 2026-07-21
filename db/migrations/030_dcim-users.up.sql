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


-- Statements generated automatically, please review:
ALTER TABLE dcim.users OWNER TO fun_owner;
