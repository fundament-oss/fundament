package dcim

import (
	"context"

	"connectrpc.com/connect"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) ListRacks(ctx context.Context, req *connect.Request[dcimv1.ListRacksRequest]) (*connect.Response[dcimv1.ListRacksResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) GetRack(ctx context.Context, req *connect.Request[dcimv1.GetRackRequest]) (*connect.Response[dcimv1.GetRackResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) CreateRack(ctx context.Context, req *connect.Request[dcimv1.CreateRackRequest]) (*connect.Response[dcimv1.CreateRackResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) UpdateRack(ctx context.Context, req *connect.Request[dcimv1.UpdateRackRequest]) (*connect.Response[dcimv1.UpdateRackResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) DeleteRack(ctx context.Context, req *connect.Request[dcimv1.DeleteRackRequest]) (*connect.Response[dcimv1.DeleteRackResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}
