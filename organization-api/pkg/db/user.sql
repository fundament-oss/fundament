-- name: UserCreate :one
INSERT INTO tenant.users (name, email)
VALUES (@email::text, @email::text)
RETURNING id, name, external_ref, email, created;

-- name: UserFindByEmail :one
SELECT id, name, external_ref, email, created
FROM tenant.users
WHERE email = @email::text
    AND deleted IS NULL
LIMIT 1;
