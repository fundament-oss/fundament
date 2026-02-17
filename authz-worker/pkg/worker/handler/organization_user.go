package handler

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	db "github.com/fundament-oss/fundament/authz-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/dbconst"
)

// OrganizationUser syncs a user's organization membership and role to OpenFGA.
func (h *Handler) OrganizationUser(ctx context.Context, qtx *db.Queries, organizationUserID uuid.UUID) error {
	orgUser, err := qtx.GetOrganizationUserByID(ctx, db.GetOrganizationUserByIDParams{ID: organizationUserID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("organization user not found: %s", organizationUserID)
		}

		return fmt.Errorf("get organization user: %w", err)
	}

	h.logger.DebugContext(ctx, "handle organization user", "organization_user", orgUser)

	user := authz.User(orgUser.UserID)
	org := authz.Organization(orgUser.OrganizationID)

	// Remove conflicting role tuples first
	if err := h.deleteTuplesIfExist(ctx,
		tupleDelete(user, authz.ActionAdmin, org),
		tupleDelete(user, authz.ActionViewer, org),
	); err != nil {
		return err
	}

	if orgUser.Deleted.Valid {
		return nil
	}

	// Only write tuples for accepted memberships
	if orgUser.Status != dbconst.OrganizationsUserStatus_Accepted {
		return nil
	}

	var action authz.ActionName

	switch orgUser.Permission {
	case dbconst.OrganizationsUserPermission_Admin:
		action = authz.ActionAdmin
	case dbconst.OrganizationsUserPermission_Viewer:
		action = authz.ActionViewer
	default:
		panic(fmt.Sprintf("unknown organization user permission: %s", orgUser.Permission))
	}

	return h.writeTuples(ctx, tuple(user, action, org))
}
