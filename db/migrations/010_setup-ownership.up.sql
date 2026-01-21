-- Statements generated automatically, please review:
ALTER SCHEMA tenant OWNER TO fun_owner;
ALTER SCHEMA zappstore OWNER TO fun_owner;
ALTER TABLE tenant.clusters OWNER TO fun_owner;
ALTER TABLE tenant.namespaces OWNER TO fun_owner;
ALTER TABLE tenant.node_pools OWNER TO fun_owner;
ALTER TABLE tenant.organizations OWNER TO fun_owner;
ALTER TABLE tenant.projects OWNER TO fun_owner;
ALTER TABLE tenant.users OWNER TO fun_owner;
ALTER TABLE zappstore.categories OWNER TO fun_owner;
ALTER TABLE zappstore.categories_plugins OWNER TO fun_owner;
ALTER TABLE zappstore.installs OWNER TO fun_owner;
ALTER TABLE zappstore.plugin_documentation_links OWNER TO fun_owner;
ALTER TABLE zappstore.plugins OWNER TO fun_owner;
ALTER TABLE zappstore.plugins_tags OWNER TO fun_owner;
ALTER TABLE zappstore.preset_plugins OWNER TO fun_owner;
ALTER TABLE zappstore.presets OWNER TO fun_owner;
ALTER TABLE zappstore.tags OWNER TO fun_owner;

GRANT USAGE
   ON SCHEMA tenant
   TO fun_fundament_api;

GRANT USAGE
   ON SCHEMA zappstore
   TO fun_fundament_api;

GRANT USAGE
   ON SCHEMA tenant
   TO fun_authn_api;


/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "tenant"."clusters" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."clusters" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "tenant"."clusters" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "tenant"."namespaces" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."namespaces" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "tenant"."namespaces" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "tenant"."node_pools" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."node_pools" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "tenant"."node_pools" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "tenant"."organizations" TO "fun_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."organizations" TO "fun_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "tenant"."organizations" TO "fun_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "tenant"."organizations" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."organizations" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "tenant"."organizations" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "tenant"."projects" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."projects" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "tenant"."projects" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "tenant"."users" TO "fun_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."users" TO "fun_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "tenant"."users" TO "fun_authn_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "tenant"."users" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "tenant"."users" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "tenant"."users" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "zappstore"."categories" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "zappstore"."categories" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "zappstore"."categories" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "zappstore"."categories_plugins" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "zappstore"."categories_plugins" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "zappstore"."categories_plugins" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "zappstore"."installs" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "zappstore"."installs" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "zappstore"."installs" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "zappstore"."plugin_documentation_links" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "zappstore"."plugin_documentation_links" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "zappstore"."plugin_documentation_links" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "zappstore"."plugins" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "zappstore"."plugins" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "zappstore"."plugins" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "zappstore"."plugins_tags" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "zappstore"."plugins_tags" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "zappstore"."plugins_tags" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "zappstore"."preset_plugins" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "zappstore"."preset_plugins" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "zappstore"."preset_plugins" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "zappstore"."presets" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "zappstore"."presets" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "zappstore"."presets" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT INSERT ON "zappstore"."tags" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT SELECT ON "zappstore"."tags" TO "fun_fundament_api";

/* Hazards:
 - AUTHZ_UPDATE: Granting privileges could allow unauthorized access to data.
*/
GRANT UPDATE ON "zappstore"."tags" TO "fun_fundament_api";
