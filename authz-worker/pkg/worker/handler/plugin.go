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

// Plugin syncs a plugin's organization ownership to OpenFGA.
func (h *Handler) Plugin(ctx context.Context, qtx *db.Queries, pluginID uuid.UUID) error {
	plugin, err := qtx.GetPluginByID(ctx, db.GetPluginByIDParams{ID: pluginID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("plugin not found: %s", pluginID)
		}

		return fmt.Errorf("get plugin: %w", err)
	}

	h.logger.DebugContext(ctx, "handle plugin", "plugin", plugin)

	orgObj := authz.Organization(plugin.OrganizationID)
	pluginObj := authz.Plugin(plugin.ID)

	if plugin.Deleted.Valid {
		return h.deleteTuplesIfExist(ctx,
			tupleDelete(orgObj, authz.ActionOwner, pluginObj),
		)
	}

	return h.writeTuplesIfNotExist(ctx, tuple(orgObj, authz.ActionOwner, pluginObj))
}
