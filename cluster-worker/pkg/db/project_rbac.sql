-- name: ProjectRoleBindingListForCluster :many
-- Desired project RoleBindings on a cluster (FUN-7): one row per active
-- project member and active namespace of that member's project. Accepted org
-- admins are excluded: their ClusterRoleBinding already grants cluster-admin.
-- The namespace id (not its cluster-side name) is returned; the converger maps
-- it to the live namespace via the fundament.io/namespace-id label, which also
-- skips namespaces not yet created on the shoot.
SELECT
    tenant.project_members.user_id,
    tenant.namespaces.id AS namespace_id,
    tenant.project_members.role
FROM tenant.project_members
JOIN tenant.projects ON tenant.projects.id = tenant.project_members.project_id
JOIN tenant.namespaces ON tenant.namespaces.project_id = tenant.projects.id
JOIN tenant.clusters ON tenant.clusters.id = tenant.projects.cluster_id
JOIN tenant.users ON tenant.users.id = tenant.project_members.user_id
WHERE tenant.projects.cluster_id = @cluster_id
    AND tenant.project_members.deleted IS NULL
    AND tenant.projects.deleted IS NULL
    AND tenant.namespaces.deleted IS NULL
    AND tenant.users.deleted IS NULL
    AND NOT EXISTS (
        SELECT 1 FROM tenant.organizations_users
        WHERE tenant.organizations_users.organization_id = tenant.clusters.organization_id
            AND tenant.organizations_users.user_id = tenant.project_members.user_id
            AND tenant.organizations_users.permission = 'admin'
            AND tenant.organizations_users.status = 'accepted'
            AND tenant.organizations_users.deleted IS NULL
    );
