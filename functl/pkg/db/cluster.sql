-- name: ClusterCreate :one
INSERT INTO tenant.clusters (organization_id, name, region, kubernetes_version, status)
SELECT o.id, @name::text, @region::text, @kubernetes_version::text, 'unspecified'
FROM tenant.organizations o
WHERE o.name = @organization_name::text
RETURNING id, organization_id, name, region, kubernetes_version, status, created;

-- name: ClusterGet :one
SELECT c.id, c.name, c.region, c.kubernetes_version, c.status, c.created
FROM tenant.clusters c
JOIN tenant.organizations o ON o.id = c.organization_id
WHERE o.name = @organization_name::text
  AND c.name = @cluster_name::text
  AND c.deleted IS NULL;

-- name: ClusterList :many
SELECT c.id, c.name, c.region, c.kubernetes_version, c.status, c.created
FROM tenant.clusters c
JOIN tenant.organizations o ON o.id = c.organization_id
WHERE o.name = @organization_name::text AND c.deleted IS NULL
ORDER BY c.created DESC;

-- name: ClusterUpdate :one
UPDATE tenant.clusters
SET kubernetes_version = @kubernetes_version::text
FROM tenant.organizations o
WHERE tenant.clusters.organization_id = o.id
  AND o.name = @organization_name::text
  AND tenant.clusters.name = @cluster_name::text
  AND tenant.clusters.deleted IS NULL
RETURNING tenant.clusters.id, tenant.clusters.name, tenant.clusters.region,
          tenant.clusters.kubernetes_version, tenant.clusters.status, tenant.clusters.created;

-- name: ClusterDelete :execrows
UPDATE tenant.clusters
SET deleted = now()
FROM tenant.organizations o
WHERE tenant.clusters.organization_id = o.id
  AND o.name = @organization_name::text
  AND tenant.clusters.name = @cluster_name::text
  AND tenant.clusters.deleted IS NULL;
