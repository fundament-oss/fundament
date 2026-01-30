-- name: APIKeyCreate :one
INSERT INTO authn.api_keys (organization_id, user_id, name, token_hash, token_prefix, expires)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id;

-- name: APIKeyGetByID :one
SELECT id, organization_id, user_id, name, token_prefix, expires, revoked, last_used, created, deleted
FROM authn.api_keys
WHERE id = $1 AND deleted IS NULL;

-- name: APIKeyListByOrganizationID :many
SELECT id, organization_id, user_id, name, token_prefix, expires, revoked, last_used, created, deleted
FROM authn.api_keys
WHERE organization_id = $1 AND user_id = $2 AND deleted IS NULL
ORDER BY created DESC;

-- name: APIKeyRevoke :execrows
UPDATE authn.api_keys
SET revoked = NOW()
WHERE id = $1 AND deleted IS NULL AND revoked IS NULL;

-- name: APIKeyDelete :execrows
UPDATE authn.api_keys
SET deleted = NOW()
WHERE id = $1 AND deleted IS NULL;
