
-- name: OrganizationGetByID :one
-- Membership is asserted here rather than left to RLS: the plugin-publisher
-- policy also grants SELECT on this table, and this endpoint must not expose it.
SELECT id, name, alias, created
FROM tenant.organizations
WHERE id = $1
  AND (tenant.organizations.id = authn.current_organization_id()
       OR authn.is_organization_member(tenant.organizations.id));

-- name: OrganizationUpdate :one
UPDATE tenant.organizations
SET alias = $2
WHERE id = $1
RETURNING id, name, alias, created;

-- name: OrganizationList :many
-- Membership is asserted here rather than left to RLS: the plugin-publisher
-- policy also grants SELECT on this table, and listing must stay member-scoped.
SELECT id, name, alias, created
FROM tenant.organizations
WHERE tenant.organizations.id = authn.current_organization_id()
   OR authn.is_organization_member(tenant.organizations.id)
ORDER BY created;
