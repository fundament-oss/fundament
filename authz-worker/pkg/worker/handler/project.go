package handler

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	db "github.com/fundament-oss/fundament/authz-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/common/authz"
)

// Project syncs a project's organization relationship to OpenFGA.
func (h *Handler) Project(ctx context.Context, qtx *db.Queries, projectID uuid.UUID) error {
	project, err := qtx.GetProjectByID(ctx, db.GetProjectByIDParams{ID: projectID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("project not found: %s", projectID)
		}

		return fmt.Errorf("get project: %w", err)
	}

	h.logger.DebugContext(ctx, "handle project", "project", project)

	orgObj := authz.Organization(project.OrganizationID)
	projectObj := authz.Project(project.ID)

	if project.Deleted.Valid {
		return h.deleteTuplesIfExist(ctx,
			tupleDelete(orgObj, authz.ActionOwner, projectObj),
		)
	}

	return h.writeTuples(ctx, tuple(orgObj, authz.ActionOwner, projectObj))
}
