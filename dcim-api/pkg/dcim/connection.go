package dcim

import (
	"context"

	"connectrpc.com/connect"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) CreatePhysicalConnection(ctx context.Context, req *connect.Request[dcimv1.CreatePhysicalConnectionRequest]) (*connect.Response[dcimv1.CreatePhysicalConnectionResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) GetPhysicalConnection(ctx context.Context, req *connect.Request[dcimv1.GetPhysicalConnectionRequest]) (*connect.Response[dcimv1.GetPhysicalConnectionResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) UpdatePhysicalConnection(ctx context.Context, req *connect.Request[dcimv1.UpdatePhysicalConnectionRequest]) (*connect.Response[dcimv1.UpdatePhysicalConnectionResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) DeletePhysicalConnection(ctx context.Context, req *connect.Request[dcimv1.DeletePhysicalConnectionRequest]) (*connect.Response[dcimv1.DeletePhysicalConnectionResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) ListConnectionsByPlacement(ctx context.Context, req *connect.Request[dcimv1.ListConnectionsByPlacementRequest]) (*connect.Response[dcimv1.ListConnectionsByPlacementResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}
