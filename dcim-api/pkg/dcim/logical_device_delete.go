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

func (s *Server) DeleteDevice(
	ctx context.Context,
	req *connect.Request[dcimv1.DeleteDeviceRequest],
) (*connect.Response[emptypb.Empty], error) {
	deviceID := uuid.MustParse(req.Msg.GetId())

	rowsAffected, err := s.queries.LogicalDeviceDelete(ctx, db.LogicalDeviceDeleteParams{ID: deviceID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete device: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("device not found"))
	}

	s.logger.InfoContext(ctx, "device deleted", "device_id", deviceID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
