SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

CREATE SCHEMA "authn";

ALTER SCHEMA authn OWNER TO fun_owner;

GRANT USAGE
   ON SCHEMA authn
   TO fun_authn_api;

GRANT USAGE
   ON SCHEMA authn
   TO fun_fundament_api;

CREATE TABLE "authn"."api_keys" (
	"id" uuid DEFAULT uuidv7() NOT NULL,
	"organization_id" uuid NOT NULL,
	"user_id" uuid NOT NULL,
	"name" text COLLATE "pg_catalog"."default" NOT NULL,
	"token_hash" bytea NOT NULL,
	"token_prefix" text COLLATE "pg_catalog"."default" NOT NULL,
	"expires" timestamp with time zone,
  "revoked" timestamp with time zone,
	"last_used" timestamp with time zone,
	"created" timestamp with time zone DEFAULT now() NOT NULL,
	"deleted" timestamp with time zone
);

ALTER TABLE authn.api_keys OWNER TO fun_owner;

CREATE POLICY "api_keys_organization_policy" ON "authn"."api_keys"
	AS PERMISSIVE
	FOR ALL
	TO fun_fundament_api
	USING (((organization_id = (current_setting('app.current_organization_id'::text))::uuid) AND (user_id = (current_setting('app.current_user_id'::text))::uuid)));

ALTER TABLE "authn"."api_keys" ENABLE ROW LEVEL SECURITY;

CREATE UNIQUE INDEX api_keys_pk ON authn.api_keys USING btree (id);

ALTER TABLE "authn"."api_keys" ADD CONSTRAINT "api_keys_pk" PRIMARY KEY USING INDEX "api_keys_pk";

CREATE UNIQUE INDEX api_keys_uq_name ON authn.api_keys USING btree (organization_id, name, deleted) NULLS NOT DISTINCT;

ALTER TABLE "authn"."api_keys" ADD CONSTRAINT "api_keys_uq_name" UNIQUE USING INDEX "api_keys_uq_name";

CREATE UNIQUE INDEX api_keys_uq_token_hash ON authn.api_keys USING btree (token_hash);

ALTER TABLE "authn"."api_keys" ADD CONSTRAINT "api_keys_uq_token_hash" UNIQUE USING INDEX "api_keys_uq_token_hash";

ALTER TABLE "authn"."api_keys" ADD CONSTRAINT "api_keys_fk_organization" FOREIGN KEY (organization_id) REFERENCES tenant.organizations(id) NOT VALID;

ALTER TABLE "authn"."api_keys" VALIDATE CONSTRAINT "api_keys_fk_organization";

ALTER TABLE "authn"."api_keys" ADD CONSTRAINT "api_keys_fk_user" FOREIGN KEY (user_id) REFERENCES tenant.users(id) NOT VALID;

ALTER TABLE "authn"."api_keys" VALIDATE CONSTRAINT "api_keys_fk_user";

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authn.api_key_get_by_hash(p_token_hash bytea)
 RETURNS authn.api_keys
 LANGUAGE plpgsql
 SECURITY DEFINER COST 10
AS $function$
DECLARE
	result authn.api_keys;
	key_record authn.api_keys;
BEGIN
	SELECT * INTO key_record FROM authn.api_keys WHERE token_hash = p_token_hash;

	IF NOT FOUND THEN
		RETURN NULL;
	END IF;

	IF key_record.deleted IS NOT NULL THEN
		RAISE EXCEPTION 'API key has been deleted' USING HINT = 'api_key_deleted';
	END IF;

	IF key_record.revoked IS NOT NULL THEN
		RAISE EXCEPTION 'API key has been revoked' USING HINT = 'api_key_revoked';
	END IF;

	IF key_record.expires IS NOT NULL AND key_record.expires <= NOW() THEN
		RAISE EXCEPTION 'API key has expired' USING HINT = 'api_key_expired';
	END IF;

	UPDATE authn.api_keys
	SET last_used = NOW()
	WHERE id = key_record.id
	RETURNING * INTO result;

	RETURN result;
END;
$function$
;




-- Statements generated automatically, please review:
ALTER FUNCTION authn.api_key_get_by_hash(p_token_hash bytea) OWNER TO fun_owner;

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "authn"."api_keys" TO "fun_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "authn"."api_keys" TO "fun_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "authn"."api_keys" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "authn"."api_keys" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "authn"."api_keys" TO "fun_fundament_api";
