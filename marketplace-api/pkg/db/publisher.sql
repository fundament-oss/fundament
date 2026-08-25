-- name: PublisherList :many
SELECT tenant.organizations.id, tenant.organizations.name, tenant.organizations.alias
FROM tenant.organizations
ORDER BY tenant.organizations.alias, tenant.organizations.name;
