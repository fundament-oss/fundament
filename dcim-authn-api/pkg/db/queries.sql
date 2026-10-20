-- name: UserUpsert :one
-- The staff directory entry for an authenticated IdP identity, provisioned
-- just-in-time at login: the staff IdP (dex-dcim) is the access boundary, so
-- anyone who authenticates there belongs in the roster. The returned id is the
-- DCIM JWT subject; the external reference never leaves this service.
--
-- The update is a patch, not a replace: a connector that omits the name or
-- email claim must not erase what the directory already holds, so an absent
-- value keeps the existing one.
--
-- The conflict target is partial (deleted IS NULL), so a soft-deleted user who
-- still authenticates at the IdP gets a fresh row and id; their history stays
-- on the dead row. Accepted: the IdP is the access boundary, and revoking
-- access means removing the account there, not soft-deleting the row here.
INSERT INTO dcim.users (name, external_ref, email)
VALUES ($1, $2, $3)
ON CONFLICT (external_ref) WHERE deleted IS NULL
DO UPDATE SET
	name = COALESCE(NULLIF(EXCLUDED.name, ''), users.name),
	email = COALESCE(EXCLUDED.email, users.email)
RETURNING id, name;

-- name: UserGetByID :one
-- Refresh resolves the token subject back onto the directory: a subject that
-- no longer resolves (a soft-deleted user, or a pre-rollover token whose
-- subject is an external reference rather than dcim.users.id) must not have
-- its session extended.
SELECT id, name
FROM dcim.users
WHERE id = $1 AND deleted IS NULL;
