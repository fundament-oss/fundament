-- name: SetOrganizationContext :exec
SELECT set_config('app.current_organization_id', $1, false);

-- name: SetUserContext :exec
SELECT set_config('app.current_user_id', $1, false);

-- name: ResetOrganizationContext :exec
RESET app.current_organization_id;

-- name: ResetUserContext :exec
RESET app.current_user_id;
