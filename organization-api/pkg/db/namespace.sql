-- name: NamespaceListByClusterID :many
SELECT
  namespaces.id,
  namespaces.project_id,
  namespaces.name,
  namespaces.created,
  namespaces.deleted,
  projects.cluster_id
FROM tenant.namespaces
JOIN tenant.projects
  ON projects.id = namespaces.project_id
WHERE projects.cluster_id = $1
  AND namespaces.deleted IS NULL
ORDER BY namespaces.name ASC;

-- name: NamespaceGetByID :one
SELECT
  namespaces.id,
  namespaces.project_id,
  namespaces.name,
  namespaces.created,
  namespaces.deleted,
  projects.cluster_id
FROM tenant.namespaces
JOIN tenant.projects
  ON projects.id = namespaces.project_id
WHERE namespaces.id = $1
  AND namespaces.deleted IS NULL;

-- name: NamespaceCreate :one
INSERT INTO tenant.namespaces (project_id, name)
VALUES ($1, $2)
RETURNING id;

-- name: NamespaceDelete :execrows
UPDATE tenant.namespaces
SET deleted = NOW()
WHERE id = $1 AND deleted IS NULL;

-- name: NamespaceListByProjectID :many
SELECT
  namespaces.id,
  namespaces.project_id,
  namespaces.name,
  namespaces.created,
  namespaces.deleted,
  projects.cluster_id
FROM tenant.namespaces
JOIN tenant.projects
  ON projects.id = namespaces.project_id
WHERE namespaces.project_id = $1
  AND namespaces.deleted IS NULL
ORDER BY namespaces.name ASC;

-- name: NamespaceGetByProjectAndName :one
SELECT
  namespaces.id,
  namespaces.project_id,
  namespaces.name,
  namespaces.created,
  namespaces.deleted,
  projects.cluster_id
FROM tenant.namespaces
JOIN tenant.projects
  ON projects.id = namespaces.project_id
JOIN tenant.clusters
  ON clusters.id = projects.cluster_id
WHERE clusters.name = sqlc.arg('cluster_name')
  AND projects.name = sqlc.arg('project_name')
  AND namespaces.name = sqlc.arg('namespace_name')
  AND namespaces.deleted IS NULL
  AND projects.deleted IS NULL
  AND clusters.deleted IS NULL;
