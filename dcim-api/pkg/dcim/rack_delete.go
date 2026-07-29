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

func (s *Server) DeleteRack(
	ctx context.Context,
	req *dcimv1.DeleteRackRequest,
) (*emptypb.Empty, error) {
	rackID := uuid.MustParse(req.GetId())

	rowsAffected, err := s.queries.RackDelete(ctx, db.RackDeleteParams{ID: rackID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete rack: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("rack not found"))
	}

	s.logger.InfoContext(ctx, "rack deleted", "rack_id", rackID)

	return &emptypb.Empty{}, nil
}
