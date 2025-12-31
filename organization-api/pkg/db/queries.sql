-- name: TenantGetByID :one
SELECT id, name, created
FROM organization.tenants
WHERE id = $1;

-- name: TenantUpdate :one
UPDATE organization.tenants
SET name = $2
WHERE id = $1
RETURNING id, name, created;

-- name: ClusterListByTenantID :many
SELECT id, tenant_id, name, region, kubernetes_version, status, created, deleted
FROM organization.clusters
WHERE tenant_id = $1 AND deleted IS NULL
ORDER BY created DESC;

-- name: ClusterGetByID :one
SELECT id, tenant_id, name, region, kubernetes_version, status, created, deleted
FROM organization.clusters
WHERE id = $1 AND deleted IS NULL;

-- name: ClusterCreate :one
INSERT INTO organization.clusters (id, tenant_id, name, region, kubernetes_version, status)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, tenant_id, name, region, kubernetes_version, status, created, deleted;

-- name: ClusterUpdate :one
UPDATE organization.clusters
SET kubernetes_version = COALESCE(sqlc.narg('kubernetes_version'), kubernetes_version),
    status = COALESCE(sqlc.narg('status'), status)
WHERE id = $1 AND deleted IS NULL
RETURNING id, tenant_id, name, region, kubernetes_version, status, created, deleted;

-- name: ClusterDelete :exec
UPDATE organization.clusters
SET deleted = NOW()
WHERE id = $1 AND deleted IS NULL;
