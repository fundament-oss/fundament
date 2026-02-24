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
    EXISTS (
        SELECT 1 FROM tenant.cluster_outbox
        WHERE tenant.cluster_outbox.cluster_id = tenant.clusters.id
          AND tenant.cluster_outbox.status = 'completed'
    ) -- Manifest was applied
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
    EXISTS (
        SELECT 1 FROM tenant.cluster_outbox
        WHERE tenant.cluster_outbox.cluster_id = tenant.clusters.id
          AND tenant.cluster_outbox.status = 'completed'
    ) -- Delete was synced
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
