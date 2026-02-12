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

// Install syncs an install's cluster relationship to OpenFGA.
func (h *Handler) Install(ctx context.Context, qtx *db.Queries, installID uuid.UUID) error {
	install, err := qtx.GetInstallByID(ctx, db.GetInstallByIDParams{ID: installID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("install not found: %s", installID)
		}

		return fmt.Errorf("get install: %w", err)
	}

	h.logger.DebugContext(ctx, "handle install", "install", install)

	clusterObj := authz.Cluster(install.ClusterID)
	installObj := authz.Install(install.ID)

	if install.Deleted.Valid {
		return h.deleteTuplesIfExist(ctx,
			tupleDelete(clusterObj, authz.ActionParent, installObj),
		)
	}

	return h.writeTuples(ctx, tuple(clusterObj, authz.ActionParent, installObj))
}
