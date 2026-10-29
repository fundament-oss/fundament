SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For drops, this means you need to ensure that all functions this function depends on are dropped after this statement.
*/
DROP FUNCTION "authn"."api_key_get_by_hash"(p_token_hash bytea);

-- Written by hand: trek emits CREATE OR REPLACE, which cannot change a return type.
CREATE FUNCTION authn.api_key_get_by_hash (IN p_token_hash bytea)
	RETURNS SETOF authn.api_keys
	LANGUAGE plpgsql
	VOLATILE
	CALLED ON NULL INPUT
	SECURITY DEFINER
	PARALLEL UNSAFE
	COST 10
	ROWS 1
	AS
$function$
BEGIN
	RETURN QUERY SELECT * FROM authn.api_keys WHERE token_hash = p_token_hash;
END;
$function$;

ALTER FUNCTION authn.api_key_get_by_hash(bytea) OWNER TO fun_owner;

