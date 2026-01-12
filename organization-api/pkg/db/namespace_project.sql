
-- name: NamespaceProjectAttach :exec
INSERT INTO tenant.namespaces_projects (namespace_id, project_id)
VALUES (@namespace_id::uuid, @project_id::uuid);

-- name: NamespaceProjectDetach :execrows
DELETE FROM tenant.namespaces_projects
WHERE namespace_id = @namespace_id::uuid AND project_id = @project_id::uuid;

-- name: NamespaceProjectListByNamespaceID :many
SELECT project_id::uuid, created
FROM tenant.namespaces_projects
WHERE namespace_id = @namespace_id::uuid
ORDER BY created DESC;

-- name: NamespaceProjectListByProjectID :many
SELECT namespace_id::uuid, created
FROM tenant.namespaces_projects
WHERE project_id = @project_id::uuid
ORDER BY created DESC;
