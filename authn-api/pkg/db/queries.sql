-- name: UserGetByExternalRef :one
SELECT id, name, external_ref, email, created
FROM tenant.users
WHERE external_ref = $1 AND deleted IS NULL;

-- name: UserCreate :one
INSERT INTO tenant.users (name, external_ref, email)
VALUES ($1, $2, $3)
RETURNING id, name, external_ref, email, created;

-- name: UserUpdate :one
UPDATE tenant.users
SET name = $2
WHERE external_ref = $1
RETURNING id, name, external_ref, email, created;

-- name: UserUpsert :one
INSERT INTO tenant.users (name, external_ref, email)
VALUES ($1, $2, $3)
ON CONFLICT (external_ref) WHERE deleted IS NULL
DO UPDATE SET name = EXCLUDED.name, email = EXCLUDED.email
RETURNING id, name, external_ref, email, created;

-- name: UserGetByID :one
SELECT id, name, external_ref, email, created
FROM tenant.users
WHERE id = $1 AND deleted IS NULL;

-- name: UserGetByEmail :one
-- Get a user by email who has no external_ref (pending invitation).
-- The address is matched case-insensitively: the spelling someone was invited
-- at (or registered at by an operator) and the spelling their identity provider
-- reports are the same address, and missing each other here would sign the
-- invited person in as a stranger without any of their memberships.
SELECT id, name, external_ref, email, created
FROM tenant.users
WHERE lower(email) = lower(@email::text) AND external_ref IS NULL AND deleted IS NULL
ORDER BY created
LIMIT 1;

-- name: UserSetExternalRef :exec
UPDATE tenant.users SET external_ref = $2, name = $3 WHERE id = $1;

-- name: UserListOrganizations :many
-- Get the organizations a user belongs to (only accepted memberships)
SELECT
    organizations_users.organization_id,
    organizations_users.permission,
    organizations_users.status
FROM tenant.organizations_users
WHERE organizations_users.user_id = $1
    AND organizations_users.status = 'accepted'
    AND organizations_users.deleted IS NULL
ORDER BY organizations_users.created ASC;

-- name: APIKeyGetByHash :one
-- Uses SECURITY DEFINER function to bypass RLS (we don't know org_id before lookup)
SELECT id, organization_id, user_id, name, token_prefix, expires, revoked, last_used, created, deleted
FROM authn.api_key_get_by_hash($1);

-- name: APIKeyUpdateLastUsed :exec
-- Uses SECURITY DEFINER function to bypass RLS
SELECT authn.api_key_update_last_used($1);

-- name: WebSessionCreate :one
-- The first refresh token of a browser session (FUN-23). id doubles as
-- session_id: the row that starts a chain names it, and every rotation of it
-- carries that name forward, so revoking a session is one statement.
INSERT INTO authn.web_sessions (id, session_id, user_id, token_hash, token_prefix, groups, expires)
VALUES (@id, @id, @user_id, @token_hash, @token_prefix, @groups, @expires)
RETURNING id, session_id, user_id, token_prefix, groups, started, expires, created;

-- name: WebSessionGetByHash :one
-- Looked up by hash, never by id: the browser presents the token, and only its
-- hash is stored.
SELECT id, session_id, user_id, token_prefix, groups, started, expires, rotated_to, revoked, last_used, created
FROM authn.web_sessions
WHERE token_hash = @token_hash AND deleted IS NULL;

-- name: WebSessionRotate :one
-- Spends the presented token and mints its successor in one statement, so the
-- UPDATE's row lock is what serializes two refreshes racing on the same token:
-- the second finds rotated_to already set, matches no row, and the INSERT it
-- feeds writes nothing. No rows returned therefore means "that token was
-- already spent" — a replay, or a loser of the race — and never a partial
-- rotation.
WITH spent AS (
    UPDATE authn.web_sessions
    SET rotated_to = @new_id, last_used = now()
    WHERE authn.web_sessions.id = @id
        AND authn.web_sessions.rotated_to IS NULL
        AND authn.web_sessions.revoked IS NULL
        AND authn.web_sessions.deleted IS NULL
    RETURNING authn.web_sessions.session_id, authn.web_sessions.user_id, authn.web_sessions.groups, authn.web_sessions.started
)
INSERT INTO authn.web_sessions (id, session_id, user_id, token_hash, token_prefix, groups, started, expires)
SELECT @new_id, spent.session_id, spent.user_id, @token_hash, @token_prefix, spent.groups, spent.started, @expires
FROM spent
RETURNING authn.web_sessions.id, authn.web_sessions.session_id, authn.web_sessions.user_id, authn.web_sessions.token_prefix, authn.web_sessions.groups, authn.web_sessions.started, authn.web_sessions.expires, authn.web_sessions.created;

-- name: WebSessionRevoke :execrows
-- The whole chain, by its session_id: a logout ends the session rather than one
-- of its tokens, and a token presented twice compromises every token in the
-- chain. Returns the number of rows revoked, so a handler can log whether
-- anything was still live.
UPDATE authn.web_sessions
SET revoked = now()
WHERE session_id = @session_id AND revoked IS NULL AND deleted IS NULL;

-- name: ClusterGetByID :one
-- The organization a shoot workload belongs to comes from this row, never
-- from the shoot (FUN-22). Deleted clusters refuse the exchange.
SELECT id, organization_id
FROM tenant.clusters
WHERE id = $1 AND deleted IS NULL;
