-- Queries used by the cluster handler (sync, status, reconcile).

-- name: ClusterGetForSync :one
-- Get a cluster with fields needed to build a ClusterToSync for the sync handler.
SELECT clusters.id, clusters.name, clusters.region, clusters.kubernetes_version,
       clusters.deleted, clusters.organization_id,
       organizations.name AS organization_name
FROM tenant.clusters
JOIN tenant.organizations ON organizations.id = clusters.organization_id
WHERE clusters.id = @cluster_id;

-- name: ClusterMarkSynced :exec
-- Mark cluster as synced (Gardener accepted request).
UPDATE tenant.clusters
SET
    synced = now(),
    sync_claimed_at = NULL,
    sync_error = NULL
WHERE
    id = @cluster_id;

-- name: ClusterCreateSyncSucceededEvent :one
-- Insert sync_succeeded event when Gardener accepts the manifest.
INSERT INTO
    tenant.cluster_events (cluster_id, event_type, sync_action, message)
VALUES
    (
        @cluster_id,
        'sync_succeeded',
        @sync_action,
        @message
    )
RETURNING
    id;

-- name: ClusterCreateSyncFailedEvent :one
-- Insert sync_failed event for history.
INSERT INTO
    tenant.cluster_events (
        cluster_id,
        event_type,
        sync_action,
        message,
        attempt
    )
VALUES
    (
        @cluster_id,
        'sync_failed',
        @sync_action,
        @message,
        @attempt
    )
RETURNING
    id;

-- name: ClusterListActive :many
-- Used by periodic reconciliation to compare with Gardener state.
SELECT
    tenant.clusters.id,
    tenant.clusters.name,
    tenant.clusters.deleted,
    tenant.clusters.synced,
    tenant.organizations.name AS organization_name
FROM
    tenant.clusters
    JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
WHERE
    tenant.clusters.deleted IS NULL;

-- name: ClusterListNeedingStatusCheck :many
-- Get clusters where we need to check Gardener status (active clusters).
-- Polls clusters in non-terminal states: NULL (never checked), pending, progressing, error.
-- Does NOT poll clusters in terminal state: ready.
SELECT
    tenant.clusters.id,
    tenant.clusters.name,
    tenant.clusters.region,
    tenant.clusters.kubernetes_version,
    tenant.clusters.deleted,
    tenant.clusters.shoot_status,
    tenant.clusters.organization_id,
    tenant.clusters.shoot_status_updated,
    tenant.organizations.name AS organization_name
FROM
    tenant.clusters
    JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
WHERE
    tenant.clusters.synced IS NOT NULL -- Manifest was applied
    AND tenant.clusters.deleted IS NULL -- Active (not deleted)
    AND (
        tenant.clusters.shoot_status IS NULL -- Never checked
        OR tenant.clusters.shoot_status = 'pending' -- Shoot not yet visible in Gardener
        OR tenant.clusters.shoot_status = 'progressing' -- Gardener creating/updating
        OR tenant.clusters.shoot_status = 'error'
    ) -- Failed, might recover
    AND (
        tenant.clusters.shoot_status_updated IS NULL -- Never checked
        OR tenant.clusters.shoot_status_updated < now() - INTERVAL '30 seconds'
    ) -- Not checked recently
ORDER BY
    shoot_status_updated NULLS FIRST
LIMIT
    @limit_count;

-- name: ClusterListDeletedNeedingVerification :many
-- Get deleted clusters where we need to verify Shoot is actually gone from Gardener.
-- Polls until shoot_status = 'deleted' (confirmed removed).
SELECT
    tenant.clusters.id,
    tenant.clusters.name,
    tenant.clusters.region,
    tenant.clusters.kubernetes_version,
    tenant.clusters.deleted,
    tenant.clusters.shoot_status,
    tenant.clusters.organization_id,
    tenant.clusters.shoot_status_updated,
    tenant.organizations.name AS organization_name
FROM
    tenant.clusters
    JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
WHERE
    tenant.clusters.synced IS NOT NULL -- Delete was synced
    AND tenant.clusters.deleted IS NOT NULL -- Soft-deleted
    AND (
        tenant.clusters.shoot_status IS NULL
        OR tenant.clusters.shoot_status != 'deleted'
    ) -- Not yet confirmed deleted
    AND (
        tenant.clusters.shoot_status_updated IS NULL
        OR tenant.clusters.shoot_status_updated < now() - INTERVAL '30 seconds'
    )
ORDER BY
    shoot_status_updated NULLS FIRST
LIMIT
    @limit_count;

-- name: ClusterUpdateShootStatus :exec
-- Update shoot status from Gardener polling.
UPDATE tenant.clusters
SET
    shoot_status = @status,
    shoot_status_message = @message,
    shoot_status_updated = now()
WHERE
    id = @cluster_id;

-- name: ClusterCreateStatusEvent :one
-- Insert status event (only for milestone states: ready, error, deleted).
INSERT INTO
    tenant.cluster_events (cluster_id, event_type, message)
VALUES
    (@cluster_id, @event_type, @message)
RETURNING
    id;

-- name: ClusterListFailing :many
-- Used for alerting - clusters that have failed multiple times.
SELECT
    tenant.clusters.id,
    tenant.clusters.name,
    tenant.clusters.sync_error,
    tenant.clusters.sync_attempts,
    tenant.organizations.name AS organization_name
FROM
    tenant.clusters
    JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
WHERE
    tenant.clusters.sync_attempts >= @min_attempts;

-- name: ClusterListExhausted :many
-- Lists clusters that have exceeded max sync attempts.
-- Used for alerting and admin dashboards.
SELECT
    tenant.clusters.id,
    tenant.clusters.name,
    tenant.clusters.sync_error,
    tenant.clusters.sync_attempts,
    tenant.clusters.sync_claimed_at,
    tenant.organizations.name AS organization_name
FROM
    tenant.clusters
    JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
WHERE
    tenant.clusters.synced IS NULL
    AND tenant.clusters.sync_attempts >= @max_attempts;

-- name: ClusterHasActiveWithSameName :one
-- Check if there's an active (non-deleted) cluster with the same name in the same organization.
-- Used to prevent deleting a shoot that's been recreated.
SELECT
    EXISTS (
        SELECT
            1
        FROM
            tenant.clusters
            JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
        WHERE
            tenant.organizations.name = @organization_name
            AND tenant.clusters.name = @cluster_name
            AND tenant.clusters.deleted IS NULL
    ) AS EXISTS;
