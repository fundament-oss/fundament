
-- name: ClusterListByOrganizationID :many
SELECT id, organization_id, name, region, kubernetes_version, status, created, deleted
FROM tenant.clusters
WHERE organization_id = $1 AND deleted IS NULL
ORDER BY created DESC;

-- name: ClusterGetByID :one
SELECT id, organization_id, name, region, kubernetes_version, status, created, deleted
FROM tenant.clusters
WHERE id = $1 AND deleted IS NULL;

-- name: ClusterCreate :one
INSERT INTO tenant.clusters (organization_id, name, region, kubernetes_version, status)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: ClusterUpdate :exec
UPDATE tenant.clusters
SET kubernetes_version = COALESCE(sqlc.narg('kubernetes_version'), kubernetes_version)
WHERE id = $1 AND deleted IS NULL;

-- name: ClusterDelete :exec
UPDATE tenant.clusters
SET deleted = NOW()
WHERE id = $1 AND deleted IS NULL;
