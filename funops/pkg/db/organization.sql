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
-- Locks the live organization for the rest of the transaction. A cluster
-- insert still in flight holds a key share lock on this row for its foreign
-- key, so this waits for it to commit and the counts below then see it. It
-- does not stop an insert that starts after the lock: that one waits and then
-- succeeds, because the soft-deleted row still satisfies the foreign key.
SELECT id
FROM tenant.organizations
WHERE name = $1
  AND deleted IS NULL
FOR UPDATE;

-- name: OrganizationCountLiveClusters :one
-- Projects and namespaces live under clusters, so a live cluster is what still
-- depends on the organization. A deleted cluster counts until Gardener confirms
-- its shoot is gone (shoot_status = 'deleted'), as in ClusterCreate: the
-- organization name is free again once it is deleted, and the Gardener project
-- is named after it.
SELECT count(*)
FROM tenant.clusters
WHERE organization_id = @organization_id
  AND (deleted IS NULL OR shoot_status IS DISTINCT FROM 'deleted');

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
