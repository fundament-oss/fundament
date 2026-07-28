-- name: UserList :many
-- Deliberately no email. The roster is readable by every authenticated caller
-- and the assignee picker only ever renders a name, so listing it would hand
-- out the whole staff directory's addresses for no consumer. A caller reads
-- their own address through UserGetByExternalRef (GetCurrentUser) instead.
SELECT id, name
FROM dcim.users
WHERE deleted IS NULL
ORDER BY name;

-- name: UserGetByExternalRef :one
SELECT id, name, email, created
FROM dcim.users
WHERE external_ref = $1 AND deleted IS NULL;
