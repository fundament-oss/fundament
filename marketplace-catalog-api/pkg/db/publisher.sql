-- name: PublisherList :many
SELECT tenant.organizations.id, tenant.organizations.name, tenant.organizations.alias
FROM tenant.organizations
WHERE EXISTS (
  -- A publisher is an organization with a live listing. organizations_select_
  -- catalog stopped checking that: GetPluginDefinition resolves a publisher by
  -- name for plugin-controller, whose plugins are typically unpublished.
  SELECT 1
  FROM appstore.plugins
  WHERE appstore.plugins.organization_id = tenant.organizations.id
    AND EXISTS (
      SELECT 1
      FROM appstore.plugin_definitions
      WHERE appstore.plugin_definitions.plugin_id = appstore.plugins.id
        AND appstore.plugin_definitions.published IS NOT NULL
    )
)
ORDER BY tenant.organizations.alias, tenant.organizations.name;

-- name: PublisherListInstallable :many
-- install.v1's publishers: every organization owning a listing the caller may
-- install, including its own drafts. Row-level security on appstore.plugins
-- decides which listings those are, exactly as for install.v1 ListPlugins, so
-- every listed plugin's publisher resolves to a name.
SELECT tenant.organizations.id, tenant.organizations.name, tenant.organizations.alias
FROM tenant.organizations
WHERE EXISTS (
  SELECT 1
  FROM appstore.plugins
  WHERE appstore.plugins.organization_id = tenant.organizations.id
)
ORDER BY tenant.organizations.alias, tenant.organizations.name;
