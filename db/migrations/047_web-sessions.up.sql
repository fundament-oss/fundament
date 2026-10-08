SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

CREATE TABLE "authn"."web_sessions" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"session_id" uuid NOT NULL,
	"user_id" uuid NOT NULL,
	"token_hash" bytea NOT NULL,
	"token_prefix" text COLLATE "pg_catalog"."default" NOT NULL,
	"groups" text[] COLLATE "pg_catalog"."default" DEFAULT '{}'::text[] NOT NULL,
	"started" timestamp with time zone DEFAULT now() NOT NULL,
	"expires" timestamp with time zone NOT NULL,
	"rotated_to" uuid,
	"revoked" timestamp with time zone,
	"last_used" timestamp with time zone,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

GRANT INSERT ON "authn"."web_sessions" TO "fun_authn_api";

GRANT SELECT ON "authn"."web_sessions" TO "fun_authn_api";

GRANT UPDATE ON "authn"."web_sessions" TO "fun_authn_api";

CREATE UNIQUE INDEX web_sessions_pk ON authn.web_sessions USING btree (id);

ALTER TABLE "authn"."web_sessions" ADD CONSTRAINT "web_sessions_pk" PRIMARY KEY USING INDEX "web_sessions_pk";

CREATE UNIQUE INDEX web_sessions_uq_token_hash ON authn.web_sessions USING btree (token_hash);

ALTER TABLE "authn"."web_sessions" ADD CONSTRAINT "web_sessions_uq_token_hash" UNIQUE USING INDEX "web_sessions_uq_token_hash";

CREATE INDEX web_sessions_ix_session_id ON authn.web_sessions USING btree (session_id);

ALTER TABLE "authn"."web_sessions" ADD CONSTRAINT "web_sessions_fk_user" FOREIGN KEY (user_id) REFERENCES tenant.users(id) NOT VALID;

ALTER TABLE "authn"."web_sessions" VALIDATE CONSTRAINT "web_sessions_fk_user";


-- Statements generated automatically, please review:
ALTER TABLE authn.web_sessions OWNER TO fun_owner;
