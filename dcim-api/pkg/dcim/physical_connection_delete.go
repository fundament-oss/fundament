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

func (s *Server) DeletePhysicalConnection(
	ctx context.Context,
	req *dcimv1.DeletePhysicalConnectionRequest,
) (*emptypb.Empty, error) {
	connID := uuid.MustParse(req.GetId())

	rowsAffected, err := s.queries.PhysicalConnectionDelete(ctx, db.PhysicalConnectionDeleteParams{ID: connID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete physical connection: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("physical connection not found"))
	}

	s.logger.InfoContext(ctx, "physical connection deleted", "connection_id", connID)

	return &emptypb.Empty{}, nil
}
