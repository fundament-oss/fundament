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

func (s *Server) GetPhysicalConnection(
	ctx context.Context,
	req *connect.Request[dcimv1.GetPhysicalConnectionRequest],
) (*connect.Response[dcimv1.GetPhysicalConnectionResponse], error) {
	connID := uuid.MustParse(req.Msg.GetId())

	conn, err := s.queries.PhysicalConnectionGetByID(ctx, db.PhysicalConnectionGetByIDParams{ID: connID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("physical connection not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get physical connection: %w", err))
	}

	return connect.NewResponse(dcimv1.GetPhysicalConnectionResponse_builder{
		Connection: physicalConnectionFromRow(&conn),
	}.Build()), nil
}
