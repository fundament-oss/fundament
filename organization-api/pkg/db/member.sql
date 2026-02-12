
-- name: MemberList :many
SELECT
    users.id,
    organizations_users.organization_id,
    users.name,
    users.external_ref,
    users.email,
    organizations_users.role,
    organizations_users.status,
    organizations_users.created
FROM tenant.users
INNER JOIN tenant.organizations_users
    ON organizations_users.user_id = users.id
WHERE organizations_users.deleted IS NULL
    AND users.deleted IS NULL
ORDER BY organizations_users.created DESC;

-- name: MemberDelete :exec
UPDATE tenant.organizations_users
SET deleted = NOW(), status = 'revoked'
WHERE id = @id
  AND deleted IS NULL;
