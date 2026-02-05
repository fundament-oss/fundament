package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) ListInstalls(
	ctx context.Context,
	req *connect.Request[organizationv1.ListInstallsRequest],
) (*connect.Response[organizationv1.ListInstallsResponse], error) {
	clusterID := uuid.MustParse(req.Msg.ClusterId)

	installs, err := s.queries.InstallListByClusterID(ctx, db.InstallListByClusterIDParams{ClusterID: clusterID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list installs: %w", err))
	}

	result := make([]*organizationv1.Install, 0, len(installs))
	for i := range installs {
		result = append(result, installFromRow(&installs[i]))
	}

	return connect.NewResponse(&organizationv1.ListInstallsResponse{
		Installs: result,
	}), nil
}

func installFromRow(row *db.ZappstoreInstall) *organizationv1.Install {
	return &organizationv1.Install{
		Id:        row.ID.String(),
		PluginId:  row.PluginID.String(),
		Created: timestamppb.New(row.Created.Time),
	}
}
