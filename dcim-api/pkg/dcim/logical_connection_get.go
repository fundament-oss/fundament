package dcim

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) GetConnection(
	ctx context.Context,
	req *connect.Request[dcimv1.GetConnectionRequest],
) (*connect.Response[dcimv1.GetConnectionResponse], error) {
	connID := uuid.MustParse(req.Msg.GetId())

	conn, err := s.queries.LogicalConnectionGetByID(ctx, db.LogicalConnectionGetByIDParams{ID: connID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("connection not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get connection: %w", err))
	}

	return connect.NewResponse(dcimv1.GetConnectionResponse_builder{
		Connection: logicalConnectionFromRow(&conn),
	}.Build()), nil
}
