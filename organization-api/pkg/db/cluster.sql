
-- name: ClusterListByOrganizationID :many
SELECT c.id, c.organization_id, c.name, c.region, c.kubernetes_version, c.status, c.created, c.deleted,
       s.synced, s.sync_error, s.sync_attempts, s.sync_last_attempt, s.shoot_status, s.shoot_status_message, s.shoot_status_updated
FROM tenant.clusters c
LEFT JOIN tenant.cluster_sync s ON c.id = s.cluster_id
WHERE c.organization_id = $1 AND c.deleted IS NULL
ORDER BY c.created DESC;

-- name: ClusterGetByID :one
SELECT c.id, c.organization_id, c.name, c.region, c.kubernetes_version, c.status, c.created, c.deleted,
       s.synced, s.sync_error, s.sync_attempts, s.sync_last_attempt, s.shoot_status, s.shoot_status_message, s.shoot_status_updated
FROM tenant.clusters c
LEFT JOIN tenant.cluster_sync s ON c.id = s.cluster_id
WHERE c.id = $1 AND c.deleted IS NULL;

-- name: ClusterCreate :one
INSERT INTO tenant.clusters (organization_id, name, region, kubernetes_version, status)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: ClusterUpdate :execrows
UPDATE tenant.clusters
SET kubernetes_version = COALESCE(sqlc.narg('kubernetes_version'), kubernetes_version)
WHERE id = $1 AND deleted IS NULL;

-- name: ClusterDelete :execrows
UPDATE tenant.clusters
SET deleted = NOW()
WHERE id = $1 AND deleted IS NULL;
