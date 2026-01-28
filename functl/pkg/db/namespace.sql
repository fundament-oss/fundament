-- name: NamespaceCreate :one
INSERT INTO tenant.namespaces (project_id, cluster_id, name)
SELECT p.id, c.id, @name::text
FROM tenant.projects p
JOIN tenant.organizations o ON o.id = p.organization_id
JOIN tenant.clusters c ON c.organization_id = o.id AND c.name = @cluster_name::text AND c.deleted IS NULL
WHERE o.name = @organization_name::text AND p.name = @project_name::text AND p.deleted IS NULL
RETURNING id, project_id, cluster_id, name, created;

-- name: NamespaceList :many
SELECT
  n.id,
  n.name,
  p.name AS project_name,
  c.name AS cluster_name,
  n.created
FROM tenant.namespaces n
JOIN tenant.projects p ON p.id = n.project_id
JOIN tenant.clusters c ON c.id = n.cluster_id
JOIN tenant.organizations o ON o.id = p.organization_id
WHERE o.name = @organization_name::text AND n.deleted IS NULL
ORDER BY n.created DESC;

-- name: NamespaceDelete :execrows
UPDATE tenant.namespaces
SET deleted = now()
WHERE id = (
  SELECT n.id
  FROM tenant.namespaces n
  JOIN tenant.projects p ON p.id = n.project_id
  JOIN tenant.organizations o ON o.id = p.organization_id
  WHERE o.name = @organization_name::text
    AND p.name = @project_name::text
    AND n.name = @namespace_name::text
    AND n.deleted IS NULL
    AND p.deleted IS NULL
);
