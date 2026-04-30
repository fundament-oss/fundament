-- name: NamespaceListByClusterID :many
SELECT
  id,
  project_id,
  cluster_id,
  name,
  created,
  deleted
FROM tenant.namespaces
WHERE cluster_id = $1
  AND deleted IS NULL
ORDER BY name ASC;

-- name: NamespaceGetByID :one
SELECT
  id,
  project_id,
  cluster_id,
  name,
  created,
  deleted
FROM tenant.namespaces
WHERE id = $1
  AND deleted IS NULL;

-- name: NamespaceCreate :one
WITH project AS (
    SELECT cluster_id FROM tenant.projects WHERE id = @project_id AND deleted IS NULL
)
INSERT INTO tenant.namespaces (project_id, cluster_id, name)
SELECT @project_id, project.cluster_id, @name
FROM project
RETURNING id;

-- name: NamespaceDelete :execrows
UPDATE tenant.namespaces
SET deleted = NOW()
WHERE id = $1 AND deleted IS NULL;

-- name: NamespaceListByProjectID :many
SELECT
  id,
  project_id,
  cluster_id,
  name,
  created,
  deleted
FROM tenant.namespaces
WHERE project_id = $1
  AND deleted IS NULL
ORDER BY name ASC;

-- name: NamespaceGetByProjectAndName :one
SELECT
  namespaces.id,
  namespaces.project_id,
  namespaces.cluster_id,
  namespaces.name,
  namespaces.created,
  namespaces.deleted
FROM tenant.namespaces
JOIN tenant.projects
  ON projects.id = namespaces.project_id
JOIN tenant.clusters
  ON clusters.id = namespaces.cluster_id
WHERE clusters.name = sqlc.arg('cluster_name')
  AND projects.name = sqlc.arg('project_name')
  AND namespaces.name = sqlc.arg('namespace_name')
  AND namespaces.deleted IS NULL
  AND projects.deleted IS NULL
  AND clusters.deleted IS NULL;
