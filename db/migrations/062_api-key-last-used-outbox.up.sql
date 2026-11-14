SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authn.api_key_update_last_used(p_id uuid)
 RETURNS void
 LANGUAGE plpgsql
 SECURITY DEFINER COST 10
AS $function$
BEGIN
	-- Every token exchange calls this; last_used only tells a user roughly when
	-- a key was used, so it is written at most once a minute.
	UPDATE authn.api_keys SET last_used = NOW()
	WHERE id = p_id
	  AND (last_used IS NULL OR last_used < NOW() - interval '1 minute');
END;
$function$
;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION authz.api_keys_sync_trigger()
 RETURNS trigger
 LANGUAGE plpgsql
 COST 1
AS $function$
BEGIN
    -- Only the columns authz-worker's api key handler reads (GetApiKeyByID)
    -- decide a key's tuples. last_used changes on token exchanges and must
    -- not queue work for the worker.
    IF TG_OP = 'INSERT'
        OR (NEW.organization_id, NEW.user_id, NEW.expires, NEW.revoked, NEW.deleted)
            IS DISTINCT FROM (OLD.organization_id, OLD.user_id, OLD.expires, OLD.revoked, OLD.deleted) THEN
        INSERT INTO authz.outbox (api_key_id)
        VALUES (COALESCE(NEW.id, OLD.id));
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$function$
;

