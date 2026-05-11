package dcim

import (
	"context"

	"connectrpc.com/connect"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

// LogicalDesignService

func (s *Server) ListDesigns(ctx context.Context, req *connect.Request[dcimv1.ListDesignsRequest]) (*connect.Response[dcimv1.ListDesignsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) GetDesign(ctx context.Context, req *connect.Request[dcimv1.GetDesignRequest]) (*connect.Response[dcimv1.GetDesignResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) CreateDesign(ctx context.Context, req *connect.Request[dcimv1.CreateDesignRequest]) (*connect.Response[dcimv1.CreateDesignResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) UpdateDesign(ctx context.Context, req *connect.Request[dcimv1.UpdateDesignRequest]) (*connect.Response[emptypb.Empty], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) DeleteDesign(ctx context.Context, req *connect.Request[dcimv1.DeleteDesignRequest]) (*connect.Response[emptypb.Empty], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

// LogicalDeviceService

func (s *Server) ListDevices(ctx context.Context, req *connect.Request[dcimv1.ListDevicesRequest]) (*connect.Response[dcimv1.ListDevicesResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) GetDevice(ctx context.Context, req *connect.Request[dcimv1.GetDeviceRequest]) (*connect.Response[dcimv1.GetDeviceResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) CreateDevice(ctx context.Context, req *connect.Request[dcimv1.CreateDeviceRequest]) (*connect.Response[dcimv1.CreateDeviceResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) UpdateDevice(ctx context.Context, req *connect.Request[dcimv1.UpdateDeviceRequest]) (*connect.Response[emptypb.Empty], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) DeleteDevice(ctx context.Context, req *connect.Request[dcimv1.DeleteDeviceRequest]) (*connect.Response[emptypb.Empty], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

// LogicalConnectionService

func (s *Server) ListConnections(ctx context.Context, req *connect.Request[dcimv1.ListConnectionsRequest]) (*connect.Response[dcimv1.ListConnectionsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) GetConnection(ctx context.Context, req *connect.Request[dcimv1.GetConnectionRequest]) (*connect.Response[dcimv1.GetConnectionResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) CreateConnection(ctx context.Context, req *connect.Request[dcimv1.CreateConnectionRequest]) (*connect.Response[dcimv1.CreateConnectionResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) UpdateConnection(ctx context.Context, req *connect.Request[dcimv1.UpdateConnectionRequest]) (*connect.Response[emptypb.Empty], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) DeleteConnection(ctx context.Context, req *connect.Request[dcimv1.DeleteConnectionRequest]) (*connect.Response[emptypb.Empty], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

// LogicalDeviceLayoutService

func (s *Server) GetLayout(ctx context.Context, req *connect.Request[dcimv1.GetLayoutRequest]) (*connect.Response[dcimv1.GetLayoutResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) SaveLayout(ctx context.Context, req *connect.Request[dcimv1.SaveLayoutRequest]) (*connect.Response[dcimv1.SaveLayoutResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) DeleteLayout(ctx context.Context, req *connect.Request[dcimv1.DeleteLayoutRequest]) (*connect.Response[emptypb.Empty], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}
