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

func (s *Server) DeleteDesign(
	ctx context.Context,
	req *connect.Request[dcimv1.DeleteDesignRequest],
) (*connect.Response[emptypb.Empty], error) {
	designID := uuid.MustParse(req.Msg.GetId())

	rowsAffected, err := s.queries.LogicalDesignDelete(ctx, db.LogicalDesignDeleteParams{ID: designID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete design: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("design not found"))
	}

	s.logger.InfoContext(ctx, "design deleted", "design_id", designID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
