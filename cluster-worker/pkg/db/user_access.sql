-- name: ResolveUserAccess :one
-- Determines the desired access level for a user on a cluster.
-- Returns 'admin' if the user is an accepted org admin, 'member' if the user
-- is a project member on any project in the cluster, or 'none' otherwise.
-- Used by both the write path (UserSyncHandler) and read path (authn-api token endpoint).
SELECT
    CASE
        WHEN ou.permission = 'admin'
            AND ou.status = 'accepted'
            AND ou.deleted IS NULL
            THEN 'admin'
        WHEN pm.id IS NOT NULL
            AND pm.deleted IS NULL
            THEN 'member'
        ELSE 'none'
    END AS access_level
FROM tenant.clusters
LEFT JOIN tenant.organizations_users ou
    ON ou.organization_id = tenant.clusters.organization_id
    AND ou.user_id = @user_id
LEFT JOIN tenant.projects p
    ON p.cluster_id = tenant.clusters.id AND p.deleted IS NULL
LEFT JOIN tenant.project_members pm
    ON pm.project_id = p.id AND pm.user_id = @user_id
WHERE tenant.clusters.id = @cluster_id
LIMIT 1;

-- name: ListUsersForCluster :many
-- Returns all users who should have access to a cluster, with their access level.
-- Used by the reconciliation loop to compare against actual state on the shoot.
SELECT
    u.id AS user_id,
    u.email,
    CASE
        WHEN ou.permission = 'admin'
            AND ou.status = 'accepted'
            AND ou.deleted IS NULL
            THEN 'admin'
        ELSE 'member'
    END AS access_level
FROM tenant.clusters
JOIN tenant.organizations_users ou
    ON ou.organization_id = tenant.clusters.organization_id
    AND ou.status = 'accepted'
    AND ou.deleted IS NULL
JOIN tenant.users u ON u.id = ou.user_id AND u.deleted IS NULL
WHERE tenant.clusters.id = @cluster_id
    AND ou.permission = 'admin'
UNION
SELECT
    u.id AS user_id,
    u.email,
    'member' AS access_level
FROM tenant.clusters
JOIN tenant.projects p
    ON p.cluster_id = tenant.clusters.id AND p.deleted IS NULL
JOIN tenant.project_members pm
    ON pm.project_id = p.id AND pm.deleted IS NULL
JOIN tenant.users u ON u.id = pm.user_id AND u.deleted IS NULL
WHERE tenant.clusters.id = @cluster_id
    AND NOT EXISTS (
        SELECT 1 FROM tenant.organizations_users ou2
        WHERE ou2.organization_id = tenant.clusters.organization_id
            AND ou2.user_id = pm.user_id
            AND ou2.permission = 'admin'
            AND ou2.status = 'accepted'
            AND ou2.deleted IS NULL
    );
