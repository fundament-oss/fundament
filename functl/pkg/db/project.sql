-- name: ProjectCreate :one
INSERT INTO tenant.projects (organization_id, name)
SELECT o.id, @name::text
FROM tenant.organizations o
WHERE o.name = @organization_name::text
RETURNING id, organization_id, name, created;

-- name: ProjectGet :one
SELECT p.id, p.name, p.created
FROM tenant.projects p
JOIN tenant.organizations o ON o.id = p.organization_id
WHERE o.name = @organization_name::text
  AND p.name = @project_name::text
  AND p.deleted IS NULL;

-- name: ProjectList :many
SELECT p.id, p.name, p.created
FROM tenant.projects p
JOIN tenant.organizations o ON o.id = p.organization_id
WHERE o.name = @organization_name::text AND p.deleted IS NULL
ORDER BY p.created DESC;

-- name: ProjectUpdate :one
UPDATE tenant.projects
SET name = @new_name::text
FROM tenant.organizations o
WHERE tenant.projects.organization_id = o.id
  AND o.name = @organization_name::text
  AND tenant.projects.name = @project_name::text
  AND tenant.projects.deleted IS NULL
RETURNING tenant.projects.id, tenant.projects.name, tenant.projects.created;

-- name: ProjectDelete :execrows
UPDATE tenant.projects
SET deleted = now()
FROM tenant.organizations o
WHERE tenant.projects.organization_id = o.id
  AND o.name = @organization_name::text
  AND tenant.projects.name = @project_name::text
  AND tenant.projects.deleted IS NULL;
