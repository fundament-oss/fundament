-- name: OrganizationCreate :one
INSERT INTO tenant.organizations (name, alias)
VALUES ($1, $2)
RETURNING
  id,
  name,
  alias,
  created;

-- name: OrganizationList :many
SELECT
  id,
  name,
  alias,
  created
FROM tenant.organizations
WHERE deleted IS NULL
ORDER BY created DESC;

-- name: OrganizationGetIDByNameForUpdate :one
-- Locks the live organization for the rest of the transaction, so a cluster
-- cannot be created in it (the foreign key check needs a share lock on this
-- row) while it is being deleted.
SELECT id
FROM tenant.organizations
WHERE name = $1
  AND deleted IS NULL
FOR UPDATE;

-- name: OrganizationCountLiveClusters :one
-- Projects and namespaces live under clusters, so a live cluster is what still
-- depends on the organization.
SELECT count(*)
FROM tenant.clusters
WHERE organization_id = @organization_id
  AND deleted IS NULL;

-- name: OrganizationCountLivePlugins :one
SELECT count(*)
FROM appstore.plugins
WHERE organization_id = @organization_id
  AND deleted IS NULL;

-- name: OrganizationDelete :execrows
-- Soft-deletes the organization; the name is free for a new one afterwards
-- (organizations_uq_name includes deleted).
UPDATE tenant.organizations
SET deleted = now()
WHERE id = @id
  AND deleted IS NULL;

-- name: OrganizationGetIDByName :one
SELECT id
FROM tenant.organizations
WHERE name = $1
  AND deleted IS NULL;
