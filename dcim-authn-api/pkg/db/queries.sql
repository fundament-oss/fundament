-- name: UserUpsert :one
-- The staff directory entry for an authenticated IdP identity, provisioned
-- just-in-time at login: the staff IdP (dex-dcim) is the access boundary, so
-- anyone who authenticates there belongs in the roster. The returned id is the
-- DCIM JWT subject.
--
-- The update is a patch, not a replace: a connector that omits the name or
-- email claim must not erase what the directory already holds, so an absent
-- value keeps the existing one. A first login simply records what the IdP
-- sent — there is nothing to preserve yet. The absent-claim rule lives here
-- and nowhere else: the caller passes claims verbatim.
--
-- The conflict target is partial (deleted IS NULL), so a soft-deleted user who
-- still authenticates at the IdP gets a fresh row and id; their history stays
-- on the dead row. Accepted: the IdP is the access boundary, and revoking
-- access means removing the account there, not soft-deleting the row here.
INSERT INTO dcim.users (name, external_ref, email)
VALUES (sqlc.arg('name')::text, sqlc.arg('external_ref')::text, NULLIF(sqlc.arg('email')::text, ''))
ON CONFLICT (external_ref) WHERE deleted IS NULL
DO UPDATE SET
	name = COALESCE(NULLIF(EXCLUDED.name, ''), users.name),
	email = COALESCE(EXCLUDED.email, users.email)
RETURNING id, name;

-- name: UserGetByID :one
-- Resolves a token subject back onto the directory: a subject that no longer
-- resolves must not have its session extended or served.
SELECT id, name
FROM dcim.users
WHERE id = $1 AND deleted IS NULL;
