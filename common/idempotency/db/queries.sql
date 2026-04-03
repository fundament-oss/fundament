-- name: IdempotencyKeyLookup :one
SELECT
	tenant.idempotency_keys.procedure,
	tenant.idempotency_keys.request_hash,
	tenant.idempotency_keys.response_bytes,
	COALESCE(tenant.idempotency_keys.project_id, tenant.idempotency_keys.project_member_id, tenant.idempotency_keys.cluster_id, tenant.idempotency_keys.node_pool_id, tenant.idempotency_keys.namespace_id, tenant.idempotency_keys.api_key_id, tenant.idempotency_keys.install_id, tenant.idempotency_keys.organization_user_id) AS resource_id
FROM tenant.idempotency_keys
WHERE tenant.idempotency_keys.idempotency_key = $1
	AND tenant.idempotency_keys.user_id = $2
	AND tenant.idempotency_keys.expires > now();

-- name: IdempotencyKeyReserve :execrows
INSERT INTO tenant.idempotency_keys (
	idempotency_key, user_id, procedure, request_hash, expires
) VALUES (
	$1, $2, $3, $4, sqlc.arg(expires)
)
ON CONFLICT (idempotency_key, user_id) DO NOTHING;

-- name: IdempotencyKeyComplete :execrows
UPDATE tenant.idempotency_keys
SET
	response_bytes = $3,
	project_id = $4,
	project_member_id = $5,
	cluster_id = $6,
	node_pool_id = $7,
	namespace_id = $8,
	api_key_id = $9,
	install_id = $10,
	organization_user_id = $11
WHERE tenant.idempotency_keys.idempotency_key = $1
	AND tenant.idempotency_keys.user_id = $2
	AND tenant.idempotency_keys.response_bytes IS NULL;

-- Idempotency keys are ephemeral cache entries, not domain data.
-- Hard deletes are intentional here; they do not follow the soft-delete convention.

-- name: IdempotencyKeyUnreserve :execrows
DELETE FROM tenant.idempotency_keys
WHERE tenant.idempotency_keys.idempotency_key = $1
	AND tenant.idempotency_keys.user_id = $2
	AND tenant.idempotency_keys.response_bytes IS NULL;

-- name: IdempotencyKeyDeleteExpired :execrows
DELETE FROM tenant.idempotency_keys WHERE expires < now();
