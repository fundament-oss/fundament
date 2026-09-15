-- name: UserCreate :one
INSERT INTO tenant.users (name, email)
VALUES (@email::text, @email::text)
RETURNING id, name, external_ref, email, created;

-- name: UserFindByEmail :one
-- The address is matched case-insensitively, so inviting someone at an address
-- they already have an account at reaches that account instead of leaving a
-- second, unreachable one behind. An account somebody has signed in to comes
-- before an invitation still waiting to be claimed.
SELECT id, name, external_ref, email, created
FROM tenant.users
WHERE lower(email) = lower(@email::text)
    AND deleted IS NULL
ORDER BY external_ref IS NULL, created
LIMIT 1;
