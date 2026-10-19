SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT USAGE ON SCHEMA "dcim" TO "fun_dcim_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "dcim"."users" TO "fun_dcim_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "dcim"."users" TO "fun_dcim_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "dcim"."users" TO "fun_dcim_authn_api";

COMMENT ON COLUMN dcim.users.external_ref IS E'Identity-provider reference this user maps onto (the UUIDv5 dcim-authn-api derives from the IdP `sub`); dcim-authn-api resolves it to the internal id at login, so the DCIM JWT `sub` is dcim.users.id. Null for users that exist only in DCIM and cannot sign in.';

