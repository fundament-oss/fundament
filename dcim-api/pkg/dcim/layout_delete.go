package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) DeleteLayout(
	ctx context.Context,
	req *connect.Request[dcimv1.DeleteLayoutRequest],
) (*connect.Response[emptypb.Empty], error) {
	designID := uuid.MustParse(req.Msg.GetDesignId())

	err := s.queries.LogicalDeviceLayoutDeleteByDesign(ctx, db.LogicalDeviceLayoutDeleteByDesignParams{LogicalDesignID: designID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete layout: %w", err))
	}

	s.logger.InfoContext(ctx, "layout deleted", "design_id", designID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
