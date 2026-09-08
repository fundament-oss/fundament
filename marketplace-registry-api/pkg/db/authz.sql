-- The registry connects as one role and varies the caller per request through
-- these GUCs, which every appstore policy reads via
-- authn.current_organization_id(). Copied from organization-api/pkg/db/authz.sql
-- because sqlc generates per package.

-- name: SetOrganizationContext :exec
SELECT set_config('app.current_organization_id', $1, false);

-- name: SetUserContext :exec
SELECT set_config('app.current_user_id', $1, false);

-- name: ResetOrganizationContext :exec
RESET app.current_organization_id;

-- name: ResetUserContext :exec
RESET app.current_user_id;
