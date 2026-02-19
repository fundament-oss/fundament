-- name: ClusterClaimForSync :one
-- Claim an unsynced cluster using SKIP LOCKED with exponential backoff.
-- Prioritizes active clusters over deleted ones (active first for create/update,
-- then deleted for deletion sync).
-- Backoff formula: 30s * 2^(attempts-1), capped at 15 minutes.
-- Excludes clusters that have exhausted max sync attempts.
UPDATE tenant.clusters
SET
    sync_claimed_at = now()
WHERE
    id = (
        SELECT
            tenant.clusters.id
        FROM
            tenant.clusters clusters
            JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
        WHERE
            tenant.clusters.synced IS NULL
            AND tenant.clusters.sync_attempts < @max_attempts
            AND (
                tenant.clusters.sync_claimed_at IS NULL -- Never attempted
                OR tenant.clusters.sync_claimed_at < now() - LEAST(
                    INTERVAL '15 minutes', -- Max backoff cap
                    INTERVAL '30 seconds' * POWER(2, GREATEST(0, tenant.clusters.sync_attempts - 1))
                )
            )
        ORDER BY
            (tenant.clusters.deleted IS NOT NULL), -- Active clusters first (false < true)
            tenant.clusters.created
        FOR UPDATE OF
            clusters SKIP LOCKED
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
    organization_id,
    (
        SELECT
            tenant.organizations.name
        FROM
            tenant.organizations
        WHERE
            tenant.organizations.id = clusters.organization_id
    ) AS organization_name;

-- name: ClusterMarkSynced :exec
-- Mark cluster as synced (Gardener accepted request).
UPDATE tenant.clusters
SET
    synced = now(),
    sync_claimed_at = NULL,
    sync_error = NULL
WHERE
    id = @cluster_id;

-- name: ClusterMarkSyncFailed :exec
-- Mark sync as failed, increment attempts.
-- sync_claimed_at is kept (set to now()) for backoff calculation.
-- synced stays NULL so it will be retried after backoff.
UPDATE tenant.clusters
SET
    sync_claimed_at = now(),
    sync_error = @error,
    sync_attempts = sync_attempts + 1
WHERE
    id = @cluster_id;

-- name: ClusterSyncReset :exec
-- Used by reconciliation to mark a cluster for re-sync.
UPDATE tenant.clusters
SET
    synced = NULL,
    sync_claimed_at = NULL
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

-- name: ClusterGetForSync :one
-- Get a single cluster by ID with all data needed for sync.
SELECT
    tenant.clusters.id,
    tenant.clusters.name,
    tenant.clusters.region,
    tenant.clusters.kubernetes_version,
    tenant.clusters.deleted,
    tenant.clusters.organization_id,
    tenant.organizations.name AS organization_name
FROM
    tenant.clusters
    JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
WHERE
    tenant.clusters.id = @cluster_id;

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

-- name: ClusterSyncResetAttempts :exec
-- Resets sync attempts for a cluster, allowing it to be retried.
-- Used by admins to manually retry exhausted clusters.
UPDATE tenant.clusters
SET
    sync_attempts = 0,
    sync_error = NULL,
    sync_claimed_at = NULL
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
            tenant.clusters
            JOIN tenant.organizations ON tenant.organizations.id = tenant.clusters.organization_id
        WHERE
            tenant.organizations.name = @organization_name
            AND tenant.clusters.name = @cluster_name
            AND tenant.clusters.deleted IS NULL
    ) AS EXISTS;
