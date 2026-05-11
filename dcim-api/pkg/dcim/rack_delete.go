package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) DeleteRack(
	ctx context.Context,
	req *connect.Request[dcimv1.DeleteRackRequest],
) (*connect.Response[dcimv1.DeleteRackResponse], error) {
	rackID := uuid.MustParse(req.Msg.GetId())

	rowsAffected, err := s.queries.RackDelete(ctx, db.RackDeleteParams{ID: rackID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete rack: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("rack not found"))
	}

	s.logger.InfoContext(ctx, "rack deleted", "rack_id", rackID)

	return connect.NewResponse(dcimv1.DeleteRackResponse_builder{}.Build()), nil
}
