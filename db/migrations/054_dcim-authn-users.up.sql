SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT USAGE ON SCHEMA "dcim" TO "fun_dcim_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "dcim"."users" TO "fun_dcim_authn_api";

-- The writes are column-scoped to what the login upsert touches: the role must
-- not be able to set deleted (lockout) or re-point another user's external_ref
-- (identity takeover). id has a default and needs no grant.

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT("name"), UPDATE("name") ON "dcim"."users" TO "fun_dcim_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT("external_ref") ON "dcim"."users" TO "fun_dcim_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT("email"), UPDATE("email") ON "dcim"."users" TO "fun_dcim_authn_api";

COMMENT ON COLUMN dcim.users.external_ref IS E'Identity-provider reference this user maps onto (the IdP `sub` verbatim), the login upsert key. Null for users that exist only in DCIM and cannot sign in.';

