-- The install surface (install.v1) connects as fun_marketplace_install_api and
-- varies the caller's organization per request through this GUC, which every
-- install policy reads via authn.current_organization_id(). The storefront
-- pool never sets it. Copied from marketplace-registry-api/pkg/db/authz.sql
-- because sqlc generates per package.

-- name: SetOrganizationContext :exec
SELECT set_config('app.current_organization_id', $1, false);

-- name: ResetOrganizationContext :exec
RESET app.current_organization_id;
