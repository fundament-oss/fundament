package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) AddInstall(
	ctx context.Context,
	req *connect.Request[organizationv1.AddInstallRequest],
) (*connect.Response[organizationv1.AddInstallResponse], error) {
	clusterID := uuid.MustParse(req.Msg.GetClusterId())
	pluginID := uuid.MustParse(req.Msg.GetPluginId())

	if err := s.checkPermission(ctx, authz.CanCreateInstall(), authz.Cluster(clusterID)); err != nil {
		return nil, err
	}

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

	return connect.NewResponse(organizationv1.AddInstallResponse_builder{
		InstallId: installID.String(),
	}.Build()), nil
}
