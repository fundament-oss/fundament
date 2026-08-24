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

func (s *Server) DeleteRackRow(
	ctx context.Context,
	req *dcimv1.DeleteRackRowRequest,
) (*emptypb.Empty, error) {
	rackRowID := uuid.MustParse(req.GetId())

	rowsAffected, err := s.queries.RackRowDelete(ctx, db.RackRowDeleteParams{ID: rackRowID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete rack row: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("rack row not found"))
	}

	s.logger.InfoContext(ctx, "rack row deleted", "rack_row_id", rackRowID)

	return &emptypb.Empty{}, nil
}
