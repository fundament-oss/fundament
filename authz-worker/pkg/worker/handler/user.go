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

// User syncs a user's organization membership and role to OpenFGA.
func (h *Handler) User(ctx context.Context, qtx *db.Queries, userID uuid.UUID) error {
	user, err := qtx.GetUserByID(ctx, db.GetUserByIDParams{ID: userID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("user not found: %s", userID)
		}

		return fmt.Errorf("get user: %w", err)
	}

	h.logger.DebugContext(ctx, "handle user", "user", user)

	userObj := authz.User(user.ID)
	orgObj := authz.Organization(user.OrganizationID)

	// Remove all roles tuples first
	if err := h.deleteTuplesIfExist(ctx,
		tupleDelete(userObj, authz.ActionViewer, orgObj),
		tupleDelete(userObj, authz.ActionAdmin, orgObj),
	); err != nil {
		return fmt.Errorf("delete tuples if exists: %w", err)
	}

	if user.Deleted.Valid {
		return nil
	}

	var action authz.ActionName
	switch user.Role {
	case dbconst.UserRole_Admin:
		action = authz.ActionAdmin
	case dbconst.UserRole_Viewer:
		action = authz.ActionViewer
	default:
		panic(fmt.Sprintf("unknown user role: %s", user.Role))
	}

	if err := h.writeTuples(ctx, tuple(userObj, action, orgObj)); err != nil {
		return fmt.Errorf("write tuples: %w", err)
	}

	return nil
}
