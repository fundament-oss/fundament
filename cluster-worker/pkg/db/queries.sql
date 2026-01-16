-- name: ClusterClaimForSync :one
-- Claim a cluster using visibility timeout pattern with exponential backoff.
-- - sync_claimed_by IS NOT NULL: currently claimed, check visibility timeout (10 min)
-- - sync_claimed_by IS NULL with sync_claimed_at: previously failed, check backoff
-- Backoff formula: 30s * 2^(attempts-1), capped at 15 minutes.
-- Excludes clusters that have exhausted max sync attempts.
UPDATE tenant.clusters
SET
    sync_claimed_at = now(),
    sync_claimed_by = @worker_id
WHERE
    id = (
        SELECT
            c.id
        FROM
            tenant.clusters c
            JOIN tenant.organizations o ON o.id = c.organization_id
        WHERE
            c.synced IS NULL
            AND c.deleted IS NULL -- Active clusters only
            AND c.sync_attempts < @max_attempts
            AND (
                c.sync_claimed_at IS NULL -- Never attempted
                OR (
                    c.sync_claimed_by IS NOT NULL -- Currently claimed, check visibility timeout
                    AND c.sync_claimed_at < now() - INTERVAL '10 minutes'
                )
                OR (
                    c.sync_claimed_by IS NULL -- Previously failed, check exponential backoff
                    AND c.sync_claimed_at < now() - LEAST(
                        INTERVAL '15 minutes',
                        INTERVAL '30 seconds' * POWER(2, GREATEST(0, c.sync_attempts - 1))
                    )
                )
            )
        ORDER BY
            c.created
        FOR UPDATE OF
            c SKIP LOCKED
        LIMIT
            1
    )
RETURNING
    id,
    name,
    region,
    kubernetes_version,
    deleted,
    sync_attempts,
    (
        SELECT
            o.name
        FROM
            tenant.organizations o
        WHERE
            o.id = clusters.organization_id
    ) AS organization_name;

-- name: ClusterClaimDeletedForSync :one
-- Claim a deleted cluster for sync (to delete from Gardener).
-- Uses same visibility timeout pattern with exponential backoff.
UPDATE tenant.clusters
SET
    sync_claimed_at = now(),
    sync_claimed_by = @worker_id
WHERE
    id = (
        SELECT
            c.id
        FROM
            tenant.clusters c
            JOIN tenant.organizations o ON o.id = c.organization_id
        WHERE
            c.synced IS NULL
            AND c.deleted IS NOT NULL -- Deleted clusters only
            AND c.sync_attempts < @max_attempts
            AND (
                c.sync_claimed_at IS NULL -- Never attempted
                OR (
                    c.sync_claimed_by IS NOT NULL -- Currently claimed, check visibility timeout
                    AND c.sync_claimed_at < now() - INTERVAL '10 minutes'
                )
                OR (
                    c.sync_claimed_by IS NULL -- Previously failed, check exponential backoff
                    AND c.sync_claimed_at < now() - LEAST(
                        INTERVAL '15 minutes',
                        INTERVAL '30 seconds' * POWER(2, GREATEST(0, c.sync_attempts - 1))
                    )
                )
            )
        ORDER BY
            c.created
        FOR UPDATE OF
            c SKIP LOCKED
        LIMIT
            1
    )
RETURNING
    id,
    name,
    region,
    kubernetes_version,
    deleted,
    sync_attempts,
    (
        SELECT
            o.name
        FROM
            tenant.organizations o
        WHERE
            o.id = clusters.organization_id
    ) AS organization_name;

-- name: ClusterMarkSynced :exec
-- Mark cluster as synced (Gardener accepted request).
UPDATE tenant.clusters
SET
    synced = now(),
    sync_claimed_at = NULL,
    sync_claimed_by = NULL,
    sync_error = NULL
WHERE
    id = @cluster_id;

-- name: ClusterMarkSyncFailed :exec
-- Mark sync as failed, increment attempts, release claim.
-- sync_claimed_at is kept (set to now()) for backoff calculation.
-- sync_claimed_by is cleared to indicate no active claim.
-- synced stays NULL so it will be retried after backoff.
UPDATE tenant.clusters
SET
    sync_claimed_at = now(),
    sync_claimed_by = NULL,
    sync_error = @error,
    sync_attempts = sync_attempts + 1
WHERE
    id = @cluster_id;

-- name: ClusterSyncReset :exec
-- Used by reconciliation to mark a cluster for re-sync.
UPDATE tenant.clusters
SET
    synced = NULL,
    sync_claimed_at = NULL,
    sync_claimed_by = NULL
WHERE
    id = @cluster_id;

-- name: ClusterCreateSyncRequestedEvent :one
-- Insert sync_requested event when cluster is created/updated and needs sync.
-- This is created before the worker picks it up.
INSERT INTO
    tenant.cluster_events (cluster_id, event_type, sync_action)
VALUES
    (@cluster_id, 'sync_requested', @sync_action)
RETURNING
    id;

-- name: ClusterCreateSyncClaimedEvent :one
-- Insert sync_claimed event when worker claims a cluster for processing.
INSERT INTO
    tenant.cluster_events (cluster_id, event_type, sync_action, attempt)
VALUES
    (
        @cluster_id,
        'sync_claimed',
        @sync_action,
        @attempt
    )
RETURNING
    id;

-- name: ClusterCreateSyncSubmittedEvent :one
-- Insert sync_submitted event when Gardener accepts the manifest.
INSERT INTO
    tenant.cluster_events (cluster_id, event_type, sync_action, message)
VALUES
    (
        @cluster_id,
        'sync_submitted',
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

-- name: ClusterCreateStatusEvent :one
-- Insert status event (only for milestone states: ready, error, deleted).
INSERT INTO
    tenant.cluster_events (cluster_id, event_type, message)
VALUES
    (@cluster_id, @event_type, @message)
RETURNING
    id;

-- name: ClusterGetEvents :many
-- Get event history for a cluster.
SELECT
    id,
    cluster_id,
    event_type,
    created,
    sync_action,
    message,
    attempt
FROM
    tenant.cluster_events
WHERE
    cluster_id = @cluster_id
ORDER BY
    created DESC
LIMIT
    @limit_count;

-- name: ClusterListActive :many
-- Used by periodic reconciliation to compare with Gardener state.
SELECT
    c.id,
    c.name,
    c.deleted,
    c.synced,
    o.name AS organization_name
FROM
    tenant.clusters c
    JOIN tenant.organizations o ON o.id = c.organization_id
WHERE
    c.deleted IS NULL;

-- name: ClusterListFailing :many
-- Used for alerting - clusters that have failed multiple times.
SELECT
    c.id,
    c.name,
    c.sync_error,
    c.sync_attempts,
    o.name AS organization_name
FROM
    tenant.clusters c
    JOIN tenant.organizations o ON o.id = c.organization_id
WHERE
    c.sync_attempts >= @min_attempts;

-- name: ClusterListNeedingStatusCheck :many
-- Get clusters where we need to check Gardener status (active clusters).
-- Polls clusters in non-terminal states: NULL (never checked), pending, progressing, error.
-- Does NOT poll clusters in terminal state: ready.
SELECT
    c.id,
    c.name,
    c.region,
    c.kubernetes_version,
    c.deleted,
    c.shoot_status,
    o.name AS organization_name
FROM
    tenant.clusters c
    JOIN tenant.organizations o ON o.id = c.organization_id
WHERE
    c.synced IS NOT NULL -- Manifest was applied
    AND c.deleted IS NULL -- Active (not deleted)
    AND (
        c.shoot_status IS NULL -- Never checked
        OR c.shoot_status = 'pending' -- Shoot not yet visible in Gardener
        OR c.shoot_status = 'progressing' -- Gardener creating/updating
        OR c.shoot_status = 'error'
    ) -- Failed, might recover
    AND (
        c.shoot_status_updated IS NULL -- Never checked
        OR c.shoot_status_updated < now() - INTERVAL '30 seconds'
    ) -- Not checked recently
ORDER BY
    c.shoot_status_updated NULLS FIRST
LIMIT
    @limit_count;

-- name: ClusterListDeletedNeedingVerification :many
-- Get deleted clusters where we need to verify Shoot is actually gone from Gardener.
-- Polls until shoot_status = 'deleted' (confirmed removed).
SELECT
    c.id,
    c.name,
    c.region,
    c.kubernetes_version,
    c.deleted,
    c.shoot_status,
    o.name AS organization_name
FROM
    tenant.clusters c
    JOIN tenant.organizations o ON o.id = c.organization_id
WHERE
    c.synced IS NOT NULL -- Delete was synced
    AND c.deleted IS NOT NULL -- Soft-deleted
    AND (
        c.shoot_status IS NULL
        OR c.shoot_status != 'deleted'
    ) -- Not yet confirmed deleted
    AND (
        c.shoot_status_updated IS NULL
        OR c.shoot_status_updated < now() - INTERVAL '30 seconds'
    )
ORDER BY
    c.shoot_status_updated NULLS FIRST
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

-- name: ClusterGetByID :one
-- Get a single cluster by ID with sync state (for testing).
SELECT
    c.id,
    c.name,
    c.deleted,
    c.synced,
    c.sync_error,
    c.sync_attempts,
    c.sync_claimed_at,
    c.sync_claimed_by,
    c.shoot_status,
    c.shoot_status_message,
    c.shoot_status_updated,
    o.name AS organization_name
FROM
    tenant.clusters c
    JOIN tenant.organizations o ON o.id = c.organization_id
WHERE
    c.id = @cluster_id;

-- name: ClusterListExhausted :many
-- Lists clusters that have exceeded max sync attempts.
-- Used for alerting and admin dashboards.
SELECT
    c.id,
    c.name,
    c.sync_error,
    c.sync_attempts,
    c.sync_claimed_at,
    o.name AS organization_name
FROM
    tenant.clusters c
    JOIN tenant.organizations o ON o.id = c.organization_id
WHERE
    c.synced IS NULL
    AND c.sync_attempts >= @max_attempts;

-- name: ClusterSyncResetAttempts :exec
-- Resets sync attempts for a cluster, allowing it to be retried.
-- Used by admins to manually retry exhausted clusters.
UPDATE tenant.clusters
SET
    sync_attempts = 0,
    sync_error = NULL,
    sync_claimed_at = NULL,
    sync_claimed_by = NULL
WHERE
    id = @cluster_id;

-- name: ClusterHasActiveWithSameName :one
-- Check if there's an active (non-deleted) cluster with the same name in the same organization.
-- Used to prevent deleting a shoot that's been recreated.
SELECT
    EXISTS (
        SELECT
            1
        FROM
            tenant.clusters c
            JOIN tenant.organizations o ON o.id = c.organization_id
        WHERE
            o.name = @organization_name
            AND c.name = @cluster_name
            AND c.deleted IS NULL
    ) AS EXISTS;
