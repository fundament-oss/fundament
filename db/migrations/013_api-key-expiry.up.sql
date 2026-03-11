SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

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
BEGIN
	SELECT * INTO result FROM authn.api_keys WHERE token_hash = p_token_hash;

	IF NOT FOUND THEN
		RETURN NULL;
	END IF;

	RETURN result;
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authn.api_key_update_last_used(p_id uuid)
 RETURNS void
 LANGUAGE plpgsql
 SECURITY DEFINER COST 10
AS $function$
BEGIN
	UPDATE authn.api_keys SET last_used = NOW() WHERE id = p_id;
END;
$function$
;


-- Statements generated automatically, please review:
ALTER FUNCTION authn.api_key_update_last_used(p_id uuid) OWNER TO fun_owner;
