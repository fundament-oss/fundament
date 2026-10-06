-- name: OrganizationCreate :one
-- Refuses (no row) a name still held by a deleted organization that ever had a
-- cluster: its Gardener project is named after the organization and nothing
-- removes it on delete, so a new organization with that name could not create
-- a cluster.
-- Not safe against a concurrent `organization delete` of the same name: the
-- NOT EXISTS sees the old organization still live, the insert then waits on
-- organizations_uq_name until the delete commits, and succeeds. Accepted,
-- since it takes two operators acting on one name at the same moment.
INSERT INTO tenant.organizations (name, alias)
SELECT @name::text, @alias::text
WHERE NOT EXISTS (
    SELECT 1
    FROM tenant.organizations
    INNER JOIN tenant.clusters ON tenant.clusters.organization_id = tenant.organizations.id
    WHERE tenant.organizations.name = @name::text
      AND tenant.organizations.deleted IS NOT NULL
)
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
-- key, so this waits for it to commit and the counts below then see it. A
-- cluster insert that starts after the lock waits on it too, and then finds
-- the organization deleted (see ClusterCreate in organization-api). A plugin
-- insert is not stopped: it waits and then succeeds, because the soft-deleted
-- row still satisfies the foreign key.
SELECT id
FROM tenant.organizations
WHERE name = $1
  AND deleted IS NULL
FOR UPDATE;

-- name: OrganizationCountLiveClusters :one
-- Projects and namespaces live under clusters, so a live cluster is what still
-- depends on the organization. A deleted cluster counts until Gardener confirms
-- its shoot is gone (shoot_status = 'deleted'), as in ClusterCreate, so the
-- organization is not deleted while the cluster-worker is still tearing down
-- one of its shoots.
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
-- Soft-deletes the organization. organizations_uq_name includes deleted, so the
-- name is free for a new one afterwards, unless the organization ever had a
-- cluster (see OrganizationCreate).
UPDATE tenant.organizations
SET deleted = now()
WHERE id = @id
  AND deleted IS NULL;

-- name: OrganizationGetIDByName :one
SELECT id
FROM tenant.organizations
WHERE name = $1
  AND deleted IS NULL;
