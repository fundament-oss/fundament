
-- name: MemberListByOrganizationID :many
SELECT id, organization_id, name, external_id, email, role, created
FROM tenant.users
WHERE organization_id = $1 AND deleted IS NULL
ORDER BY created DESC;

-- name: MemberGetByEmail :one
SELECT id, organization_id, name, external_id, email, role, created
FROM tenant.users
WHERE email = @email::text AND organization_id = @organization_id AND deleted IS NULL;

-- name: MemberInvite :one
INSERT INTO tenant.users (organization_id, name, email, role)
VALUES ($1, $2, $2, $3)
RETURNING id, organization_id, name, external_id, email, role, created;

-- name: MemberDelete :exec
UPDATE tenant.users SET deleted = NOW() WHERE id = $1 AND organization_id = $2 AND deleted IS NULL;
