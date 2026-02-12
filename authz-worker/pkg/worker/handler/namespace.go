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

// Namespace syncs a namespace's project and cluster relationships to OpenFGA.
func (h *Handler) Namespace(ctx context.Context, qtx *db.Queries, namespaceID uuid.UUID) error {
	namespace, err := qtx.GetNamespaceByID(ctx, db.GetNamespaceByIDParams{ID: namespaceID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("namespace not found: %s", namespaceID)
		}

		return fmt.Errorf("get namespace: %w", err)
	}

	h.logger.DebugContext(ctx, "handle namespace", "namespace", namespace)

	projectObj := authz.Project(namespace.ProjectID)
	namespaceObj := authz.Namespace(namespace.ID)

	if namespace.Deleted.Valid {
		return h.deleteTuplesIfExist(ctx,
			tupleDelete(projectObj, authz.ActionParent, namespaceObj),
		)
	}

	return h.writeTuples(ctx, tuple(projectObj, authz.ActionParent, namespaceObj))
}
