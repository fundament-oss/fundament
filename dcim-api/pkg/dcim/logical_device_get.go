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

func (s *Server) GetDevice(
	ctx context.Context,
	req *connect.Request[dcimv1.GetDeviceRequest],
) (*connect.Response[dcimv1.GetDeviceResponse], error) {
	deviceID := uuid.MustParse(req.Msg.GetId())

	device, err := s.queries.LogicalDeviceGetByID(ctx, db.LogicalDeviceGetByIDParams{ID: deviceID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("device not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get device: %w", err))
	}

	return connect.NewResponse(dcimv1.GetDeviceResponse_builder{
		Device: logicalDeviceFromRow(&device),
	}.Build()), nil
}
