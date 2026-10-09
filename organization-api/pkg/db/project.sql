
-- name: ProjectList :many
SELECT id, cluster_id, name, alias, created, deleted
FROM tenant.projects
WHERE deleted IS NULL
ORDER BY created DESC;

-- name: ProjectListByClusterID :many
SELECT id, cluster_id, name, alias, created, deleted,
    (SELECT COUNT(*)
     FROM tenant.namespaces
     WHERE namespaces.project_id = projects.id AND namespaces.deleted IS NULL) AS namespace_count,
    (SELECT COUNT(*)
     FROM tenant.project_members
     WHERE project_members.project_id = projects.id AND project_members.deleted IS NULL) AS member_count
FROM tenant.projects
WHERE cluster_id = $1 AND deleted IS NULL
ORDER BY created DESC;

-- name: ProjectGetByID :one
SELECT id, cluster_id, name, alias, created, deleted
FROM tenant.projects
WHERE id = $1 AND deleted IS NULL;

-- name: ProjectGetByName :one
SELECT id, cluster_id, name, alias, created, deleted
FROM tenant.projects
WHERE name = $1 AND deleted IS NULL;

-- name: ProjectCreate :one
INSERT INTO tenant.projects (cluster_id, name, alias)
VALUES ($1, $2, $3)
RETURNING id;

-- name: ProjectUpdate :execrows
UPDATE tenant.projects
SET alias = COALESCE(sqlc.narg('alias'), alias)
WHERE id = $1 AND deleted IS NULL;

-- name: ProjectDelete :execrows
UPDATE tenant.projects
SET deleted = NOW()
WHERE id = $1 AND deleted IS NULL;

-- name: ProjectDefaultsGet :one
-- The project's own per-container resource defaults alongside its cluster's.
-- A NULL project column inherits the cluster's value; the cluster's value is
-- also the ceiling the project may not exceed.
SELECT
    tenant.projects.default_memory_request_mi,
    tenant.projects.default_memory_limit_mi,
    tenant.projects.default_cpu_request_m,
    tenant.projects.default_cpu_limit_m,
    tenant.clusters.default_memory_request_mi AS cluster_default_memory_request_mi,
    tenant.clusters.default_memory_limit_mi AS cluster_default_memory_limit_mi,
    tenant.clusters.default_cpu_request_m AS cluster_default_cpu_request_m,
    tenant.clusters.default_cpu_limit_m AS cluster_default_cpu_limit_m
FROM tenant.projects
JOIN tenant.clusters ON tenant.clusters.id = tenant.projects.cluster_id
WHERE tenant.projects.id = @id
  AND tenant.projects.deleted IS NULL;

-- name: ProjectDefaultsUpdate :execrows
-- Replaces all four defaults: a NULL argument inherits the cluster's value.
-- A value above the cluster's raises project_defaults_exceed_cluster.
UPDATE tenant.projects
SET default_memory_request_mi = @default_memory_request_mi,
    default_memory_limit_mi   = @default_memory_limit_mi,
    default_cpu_request_m     = @default_cpu_request_m,
    default_cpu_limit_m       = @default_cpu_limit_m
WHERE id = @id AND deleted IS NULL;
