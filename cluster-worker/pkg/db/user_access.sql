-- name: ResolveUserAccess :one
-- Determines the desired access level for a user on a cluster.
-- Returns 'admin' if the user is an accepted org admin, 'member' if the user
-- is a project member on any project in the cluster, or 'none' otherwise.
-- Used by the write path (UserSyncHandler).
-- NOTE: Duplicated in authn-api/pkg/db/queries.sql — keep both in sync.
SELECT
    CASE
        WHEN EXISTS (
            SELECT 1
            FROM tenant.organizations_users
            WHERE tenant.organizations_users.organization_id = tenant.clusters.organization_id
                AND tenant.organizations_users.user_id = @user_id
                AND tenant.organizations_users.permission = 'admin'
                AND tenant.organizations_users.status = 'accepted'
                AND tenant.organizations_users.deleted IS NULL
        )
            THEN 'admin'
        WHEN EXISTS (
            SELECT 1
            FROM tenant.projects
            JOIN tenant.project_members
                ON tenant.project_members.project_id = tenant.projects.id
            WHERE tenant.projects.cluster_id = tenant.clusters.id
                AND tenant.projects.deleted IS NULL
                AND tenant.project_members.user_id = @user_id
                AND tenant.project_members.deleted IS NULL
        )
            THEN 'member'
        ELSE 'none'
    END AS access_level
FROM tenant.clusters
WHERE tenant.clusters.id = @cluster_id
LIMIT 1;

-- name: ListUsersForCluster :many
-- Returns all users who should have access to a cluster, with their access level.
-- Used by the reconciliation loop to compare against actual state on the shoot.
SELECT
    tenant.users.id AS user_id,
    tenant.users.email,
    CASE
        WHEN tenant.organizations_users.permission = 'admin'
            AND tenant.organizations_users.status = 'accepted'
            AND tenant.organizations_users.deleted IS NULL
            THEN 'admin'
        ELSE 'member'
    END AS access_level
FROM tenant.clusters
JOIN tenant.organizations_users
    ON tenant.organizations_users.organization_id = tenant.clusters.organization_id
    AND tenant.organizations_users.status = 'accepted'
    AND tenant.organizations_users.deleted IS NULL
JOIN tenant.users ON tenant.users.id = tenant.organizations_users.user_id AND tenant.users.deleted IS NULL
WHERE tenant.clusters.id = @cluster_id
    AND tenant.organizations_users.permission = 'admin'
UNION
SELECT
    tenant.users.id AS user_id,
    tenant.users.email,
    'member' AS access_level
FROM tenant.clusters
JOIN tenant.projects
    ON tenant.projects.cluster_id = tenant.clusters.id AND tenant.projects.deleted IS NULL
JOIN tenant.project_members
    ON tenant.project_members.project_id = tenant.projects.id AND tenant.project_members.deleted IS NULL
JOIN tenant.users ON tenant.users.id = tenant.project_members.user_id AND tenant.users.deleted IS NULL
WHERE tenant.clusters.id = @cluster_id
    AND NOT EXISTS (
        SELECT 1 FROM tenant.organizations_users
        WHERE tenant.organizations_users.organization_id = tenant.clusters.organization_id
            AND tenant.organizations_users.user_id = tenant.project_members.user_id
            AND tenant.organizations_users.permission = 'admin'
            AND tenant.organizations_users.status = 'accepted'
            AND tenant.organizations_users.deleted IS NULL
    );
