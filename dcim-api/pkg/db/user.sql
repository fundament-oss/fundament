-- name: UserList :many
SELECT id, name, email, created
FROM dcim.users
WHERE deleted IS NULL
ORDER BY name;

-- name: UserGetByExternalRef :one
SELECT id, name, email, created
FROM dcim.users
WHERE external_ref = $1 AND deleted IS NULL;
