package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/organization-api/pkg/models"
	"github.com/fundament-oss/fundament/organization-api/pkg/organization/adapter"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *OrganizationServer) ListPlugins(
	ctx context.Context,
	req *connect.Request[organizationv1.ListPluginsRequest],
) (*connect.Response[organizationv1.ListPluginsResponse], error) {
	clusterID, err := uuid.Parse(req.Msg.ClusterId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid cluster id: %w", err))
	}

	// Verify cluster exists
	if _, err := s.queries.ClusterGetByID(ctx, clusterID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster: %w", err))
	}

	plugins, err := s.queries.PluginListByClusterID(ctx, clusterID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list plugins: %w", err))
	}

	return connect.NewResponse(&organizationv1.ListPluginsResponse{
		Plugins: adapter.FromPlugins(plugins),
	}), nil
}

func (s *OrganizationServer) AddPlugin(
	ctx context.Context,
	req *connect.Request[organizationv1.AddPluginRequest],
) (*connect.Response[organizationv1.AddPluginResponse], error) {
	clusterID, err := uuid.Parse(req.Msg.ClusterId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid cluster id: %w", err))
	}

	input := models.PluginCreate{
		PluginID: req.Msg.PluginId,
	}

	if err := s.validator.Validate(input); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	// Verify cluster exists
	if _, err := s.queries.ClusterGetByID(ctx, clusterID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster: %w", err))
	}

	params := db.PluginCreateParams{
		ClusterID: clusterID,
		PluginID:  input.PluginID,
	}

	plugin, err := s.queries.PluginCreate(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to add plugin: %w", err))
	}

	s.logger.InfoContext(ctx, "plugin added",
		"plugin_id", plugin.ID,
		"cluster_id", clusterID,
		"plugin_name", plugin.PluginID,
	)

	return connect.NewResponse(&organizationv1.AddPluginResponse{
		Plugin: adapter.FromPlugin(&plugin),
	}), nil
}

func (s *OrganizationServer) RemovePlugin(
	ctx context.Context,
	req *connect.Request[organizationv1.RemovePluginRequest],
) (*connect.Response[organizationv1.RemovePluginResponse], error) {
	pluginID, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid plugin id: %w", err))
	}

	if err := s.queries.PluginDelete(ctx, pluginID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to remove plugin: %w", err))
	}

	s.logger.InfoContext(ctx, "plugin removed", "plugin_id", pluginID)

	return connect.NewResponse(&organizationv1.RemovePluginResponse{
		Success: true,
	}), nil
}
