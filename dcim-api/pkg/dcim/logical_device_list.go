package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) ListDevices(
	ctx context.Context,
	req *connect.Request[dcimv1.ListDevicesRequest],
) (*connect.Response[dcimv1.ListDevicesResponse], error) {
	designID := uuid.MustParse(req.Msg.GetDesignId())

	rows, err := s.queries.LogicalDeviceList(ctx, db.LogicalDeviceListParams{LogicalDesignID: designID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list devices: %w", err))
	}

	devices := make([]*dcimv1.LogicalDevice, 0, len(rows))
	for _, row := range rows {
		devices = append(devices, logicalDeviceFromListRow(&row))
	}

	return connect.NewResponse(dcimv1.ListDevicesResponse_builder{
		Devices: devices,
	}.Build()), nil
}
