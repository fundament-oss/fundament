-- name: OutboxStatusByProjectID :one
SELECT status FROM authz.outbox WHERE project_id = $1 ORDER BY created DESC LIMIT 1;

-- name: OutboxStatusByProjectMemberID :one
SELECT status FROM authz.outbox WHERE project_member_id = $1 ORDER BY created DESC LIMIT 1;

-- name: OutboxStatusByClusterID :one
SELECT status FROM authz.outbox WHERE cluster_id = $1 ORDER BY created DESC LIMIT 1;

-- name: OutboxStatusByNodePoolID :one
SELECT status FROM authz.outbox WHERE node_pool_id = $1 ORDER BY created DESC LIMIT 1;

-- name: OutboxStatusByNamespaceID :one
SELECT status FROM authz.outbox WHERE namespace_id = $1 ORDER BY created DESC LIMIT 1;

-- name: OutboxStatusByApiKeyID :one
SELECT status FROM authz.outbox WHERE api_key_id = $1 ORDER BY created DESC LIMIT 1;

-- name: OutboxStatusByOrganizationUserID :one
SELECT status FROM authz.outbox WHERE organization_user_id = $1 ORDER BY created DESC LIMIT 1;
