package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) RemoveInstall(
	ctx context.Context,
	req *connect.Request[organizationv1.RemoveInstallRequest],
) (*connect.Response[emptypb.Empty], error) {
	installID := uuid.MustParse(req.Msg.InstallId)

	if err := s.checkPermission(ctx, authz.CanDelete(), authz.Install(installID)); err != nil {
		return nil, err
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
