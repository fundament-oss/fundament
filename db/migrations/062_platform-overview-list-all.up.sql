SET SESSION statement_timeout = 3000;
SET SESSION lock_timeout = 3000;

/* Hazards:
 - HAS_UNTRACKABLE_DEPENDENCIES: Dependencies, i.e. other functions used in the function body, of non-sql functions cannot be tracked. As a result, we cannot guarantee that function dependencies are ordered properly relative to this statement. For adds, this means you need to ensure that all functions this function depends on are created/altered before this statement.
*/
CREATE OR REPLACE FUNCTION tenant.platform_overview()
 RETURNS TABLE(organization_name text, organization_alias text, clusters bigint, projects bigint, namespaces bigint)
 LANGUAGE plpgsql
 STABLE PARALLEL SAFE SECURITY DEFINER
 SET search_path TO 'pg_catalog', 'pg_temp'
AS $function$
-- The public platform page (FUN-23) reads through here only: its role has no
-- table grants, so names and counts are all it can ever see. Counts follow the
-- chain organization -> cluster -> project -> namespace with every level live.
-- Every live organization is named, the system organization never. Until the
-- fieldlab is over listed_publicly is not read: everyone is listed by default.
-- plpgsql, not sql: a sql body is checked at CREATE, and trek's diff creates
-- functions before the columns they read, so a sql body here cannot migrate.
BEGIN
RETURN QUERY
SELECT
    tenant.organizations.name,
    tenant.organizations.alias,
    count(DISTINCT tenant.clusters.id),
    count(DISTINCT tenant.projects.id),
    count(DISTINCT tenant.namespaces.id)
FROM tenant.organizations
LEFT JOIN tenant.clusters
    ON tenant.clusters.organization_id = tenant.organizations.id
    AND tenant.clusters.deleted IS NULL
LEFT JOIN tenant.projects
    ON tenant.projects.cluster_id = tenant.clusters.id
    AND tenant.projects.deleted IS NULL
LEFT JOIN tenant.namespaces
    ON tenant.namespaces.project_id = tenant.projects.id
    AND tenant.namespaces.deleted IS NULL
WHERE tenant.organizations.deleted IS NULL
    AND tenant.organizations.id <> tenant.system_organization_id()
GROUP BY tenant.organizations.id, tenant.organizations.name, tenant.organizations.alias
ORDER BY tenant.organizations.alias, tenant.organizations.name;
END;
$function$
;

