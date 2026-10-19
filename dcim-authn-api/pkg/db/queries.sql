-- name: UserUpsert :one
-- The staff directory entry for an authenticated IdP identity, provisioned
-- just-in-time at login: the staff IdP (dex-dcim) is the access boundary, so
-- anyone who authenticates there belongs in the roster. The returned id is the
-- DCIM JWT subject; the external reference never leaves this service.
INSERT INTO dcim.users (name, external_ref, email)
VALUES ($1, $2, $3)
ON CONFLICT (external_ref) WHERE deleted IS NULL
DO UPDATE SET name = EXCLUDED.name, email = EXCLUDED.email
RETURNING id, name;

-- name: UserGetByID :one
-- Refresh resolves the token subject back onto the directory: a subject that
-- no longer resolves (a soft-deleted user, or a pre-rollover token whose
-- subject is an external reference rather than dcim.users.id) must not have
-- its session extended.
SELECT id, name
FROM dcim.users
WHERE id = $1 AND deleted IS NULL;
