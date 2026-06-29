-- name: UserList :many
SELECT id, name, email, created
FROM dcim.users
WHERE deleted IS NULL
ORDER BY name;
