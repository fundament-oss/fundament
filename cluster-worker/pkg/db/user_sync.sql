-- name: OrgUserGetForSync :one
-- Get the user_id and organization_id for an organizations_users row.
-- Used by the UserSyncHandler to fan out across all org clusters.
-- Includes soft-deleted rows: the trigger fires on delete UPDATEs too,
-- and syncUserToCluster re-resolves access via ResolveUserAccess.
SELECT
    tenant.organizations_users.user_id,
    tenant.organizations_users.organization_id
FROM tenant.organizations_users
WHERE tenant.organizations_users.id = @id;

-- name: ProjectMemberGetForSync :one
-- Get the user_id and cluster_id for a project_members row.
-- Only returns a result if the cluster is ready (shoot_status = 'ready').
-- Non-ready clusters are skipped — the event=ready trigger will handle them later.
SELECT
    tenant.project_members.user_id,
    tenant.projects.cluster_id
FROM tenant.project_members
JOIN tenant.projects ON tenant.projects.id = tenant.project_members.project_id
JOIN tenant.clusters ON tenant.clusters.id = tenant.projects.cluster_id
WHERE tenant.project_members.id = @id
  AND tenant.clusters.shoot_status = 'ready'
  AND tenant.clusters.deleted IS NULL;

-- name: ClusterListReadyForOrg :many
-- List all ready clusters for an organization.
-- Used by the UserSyncHandler when an org membership changes.
SELECT tenant.clusters.id
FROM tenant.clusters
WHERE tenant.clusters.organization_id = @organization_id
  AND tenant.clusters.shoot_status = 'ready'
  AND tenant.clusters.deleted IS NULL;

-- name: ClusterListReady :many
-- List all ready clusters (for reconciliation).
SELECT tenant.clusters.id
FROM tenant.clusters
WHERE tenant.clusters.shoot_status = 'ready'
  AND tenant.clusters.deleted IS NULL;

-- name: UserGetEmail :one
-- Get a user's email by ID.
SELECT tenant.users.email
FROM tenant.users
WHERE tenant.users.id = @id;

-- name: ClusterCreateUserSyncEvent :exec
-- Insert a user_sync_succeeded or user_sync_failed event.
INSERT INTO tenant.cluster_events (cluster_id, event_type, message)
VALUES (@cluster_id, @event_type, @message);
