package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/emptypb"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/organization-api/pkg/organization/adapter"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *OrganizationServer) ListInstalls(
	ctx context.Context,
	req *connect.Request[organizationv1.ListInstallsRequest],
) (*connect.Response[organizationv1.ListInstallsResponse], error) {
	clusterID, err := uuid.Parse(req.Msg.ClusterId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid cluster id: %w", err))
	}

	installs, err := s.queries.InstallListByClusterID(ctx, db.InstallListByClusterIDParams{ClusterID: clusterID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list installs: %w", err))
	}

	return connect.NewResponse(&organizationv1.ListInstallsResponse{
		Installs: adapter.FromInstalls(installs),
	}), nil
}

func (s *OrganizationServer) AddInstall(
	ctx context.Context,
	req *connect.Request[organizationv1.AddInstallRequest],
) (*connect.Response[organizationv1.AddInstallResponse], error) {
	clusterID, err := uuid.Parse(req.Msg.ClusterId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid cluster id: %w", err))
	}

	pluginID, err := uuid.Parse(req.Msg.PluginId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid plugin id: %w", err))
	}

	// Verify cluster exists
	if _, err := s.queries.ClusterGetByID(ctx, db.ClusterGetByIDParams{ID: clusterID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get cluster: %w", err))
	}

	params := db.InstallCreateParams{
		ClusterID: clusterID,
		PluginID:  pluginID,
	}

	installID, err := s.queries.InstallCreate(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to add install: %w", err))
	}

	s.logger.InfoContext(ctx, "install added",
		"install_id", installID,
		"cluster_id", clusterID,
		"plugin_id", pluginID,
	)

	return connect.NewResponse(&organizationv1.AddInstallResponse{
		InstallId: installID.String(),
	}), nil
}

func (s *OrganizationServer) RemoveInstall(
	ctx context.Context,
	req *connect.Request[organizationv1.RemoveInstallRequest],
) (*connect.Response[emptypb.Empty], error) {
	installID, err := uuid.Parse(req.Msg.InstallId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid install id: %w", err))
	}

	rowsAffected, err := s.queries.InstallDelete(ctx, db.InstallDeleteParams{ID: installID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to remove install: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("install not found"))
	}

	s.logger.InfoContext(ctx, "install removed", "install_id", installID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
