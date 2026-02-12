-- name: GetAndLockNextOutboxRow :one
-- Fetches the next unprocessed outbox row, skipping rows scheduled for later retry
-- or that have permanently failed.
SELECT
    id,
    project_id,
    project_member_id,
    cluster_id,
    node_pool_id,
    namespace_id,
    api_key_id,
    install_id,
    organization_user_id,
    created,
    retries
FROM authz.outbox
WHERE status IN ('pending', 'retrying')
  AND (retry_after IS NULL OR retry_after <= now())
ORDER BY created ASC
LIMIT 1
FOR NO KEY UPDATE SKIP LOCKED;

-- name: MarkOutboxRowProcessed :exec
UPDATE authz.outbox
SET processed = now(),
    status = 'completed',
    status_info = NULL
WHERE id = @id;

-- name: MarkOutboxRowRetry :one
-- Marks a row for retry with exponential backoff.
-- The backoff is calculated as: base_interval * 2^retries, capped at max_backoff.
UPDATE authz.outbox
SET retries = retries + 1,
    failed = now(),
    retry_after = now() + LEAST(
        sqlc.arg('base_interval')::interval * (1 << retries),
        @max_backoff::interval
    ),
    status = 'retrying',
    status_info = @status_info
WHERE id = @id
RETURNING retries;

-- name: MarkOutboxRowFailed :exec
-- Marks a row as permanently failed after exceeding max retries.
UPDATE authz.outbox
SET status = 'failed',
    status_info = @status_info
WHERE id = @id;

-- name: GetOrganizationUserByID :one
SELECT id, organization_id, user_id, role, status, deleted
FROM tenant.organizations_users
WHERE id = @id;

-- name: GetProjectByID :one
SELECT id, organization_id, deleted
FROM tenant.projects
WHERE id = @id;

-- name: GetProjectMemberByID :one
SELECT id, project_id, user_id, role, deleted
FROM tenant.project_members
WHERE id = @id;

-- name: GetClusterByID :one
SELECT id, organization_id, deleted
FROM tenant.clusters
WHERE id = @id;

-- name: GetNodePoolByID :one
SELECT id, cluster_id, deleted
FROM tenant.node_pools
WHERE id = @id;

-- name: GetNamespaceByID :one
SELECT id, project_id, cluster_id, deleted
FROM tenant.namespaces
WHERE id = @id;

-- name: GetApiKeyByID :one
SELECT id, organization_id, user_id, expires, revoked, deleted
FROM authn.api_keys
WHERE id = @id;

-- name: GetInstallByID :one
SELECT id, cluster_id, deleted
FROM appstore.installs
WHERE id = @id;
