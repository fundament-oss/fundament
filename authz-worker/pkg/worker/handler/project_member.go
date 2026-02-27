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

// ProjectMember syncs a project member's role to OpenFGA.
func (h *Handler) ProjectMember(ctx context.Context, qtx *db.Queries, memberID uuid.UUID) error {
	member, err := qtx.GetProjectMemberByID(ctx, db.GetProjectMemberByIDParams{ID: memberID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("project member not found: %s", memberID)
		}

		return fmt.Errorf("get project member: %w", err)
	}

	h.logger.DebugContext(ctx, "handle project member", "member", member)

	user := authz.User(member.UserID)
	project := authz.Project(member.ProjectID)
	projectMember := authz.ProjectMember(member.ID)

	// Remove conflicting role tuples first
	if err := h.deleteTuplesIfExist(ctx,
		tupleDelete(user, authz.ActionProjectAdmin, project),
		tupleDelete(user, authz.ActionProjectViewer, project),
		tupleDelete(project, authz.ActionParent, projectMember),
	); err != nil {
		return err
	}

	if member.Deleted.Valid {
		return nil
	}

	var action authz.ActionName

	switch member.Role {
	case dbconst.ProjectMemberRole_Admin:
		action = authz.ActionProjectAdmin
	case dbconst.ProjectMemberRole_Viewer:
		action = authz.ActionProjectViewer
	default:
		panic(fmt.Sprintf("unknown project member role: %s", member.Role))
	}

	return h.writeTuples(ctx,
		tuple(user, action, project),
		tuple(project, authz.ActionParent, projectMember),
	)
}
