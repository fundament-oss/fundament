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

-- name: NamespaceClusterNameTaken :one
-- Whether an active namespace of another project on the cluster already
-- materializes as the given cluster-side name "<prefix><project><separator>
-- <namespace>" (kubename.GenerateNamespace, which passes its constants in).
-- Only projects named before the separator was reserved can collide, e.g.
-- "x--y"+"z" and "x"+"y--z". Duplicates within the project itself are left to
-- namespaces_uq_name.
SELECT EXISTS (
  SELECT 1
  FROM tenant.namespaces
  JOIN tenant.projects
    ON projects.id = namespaces.project_id
  WHERE projects.cluster_id = sqlc.arg('cluster_id')
    AND projects.id <> sqlc.arg('project_id')
    AND sqlc.arg('prefix')::text || projects.name || sqlc.arg('separator')::text || namespaces.name = sqlc.arg('cluster_side_name')::text
    AND namespaces.deleted IS NULL
    AND projects.deleted IS NULL
) AS taken;

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
