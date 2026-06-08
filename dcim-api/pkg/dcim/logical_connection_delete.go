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

func (s *Server) DeleteConnection(
	ctx context.Context,
	req *connect.Request[dcimv1.DeleteConnectionRequest],
) (*connect.Response[emptypb.Empty], error) {
	connID := uuid.MustParse(req.Msg.GetId())

	rowsAffected, err := s.queries.LogicalConnectionDelete(ctx, db.LogicalConnectionDeleteParams{ID: connID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete connection: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("connection not found"))
	}

	s.logger.InfoContext(ctx, "connection deleted", "connection_id", connID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
