package dcim

import (
	"context"

	"connectrpc.com/connect"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) ListRooms(ctx context.Context, req *connect.Request[dcimv1.ListRoomsRequest]) (*connect.Response[dcimv1.ListRoomsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) GetRoom(ctx context.Context, req *connect.Request[dcimv1.GetRoomRequest]) (*connect.Response[dcimv1.GetRoomResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) CreateRoom(ctx context.Context, req *connect.Request[dcimv1.CreateRoomRequest]) (*connect.Response[dcimv1.CreateRoomResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) UpdateRoom(ctx context.Context, req *connect.Request[dcimv1.UpdateRoomRequest]) (*connect.Response[dcimv1.UpdateRoomResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) DeleteRoom(ctx context.Context, req *connect.Request[dcimv1.DeleteRoomRequest]) (*connect.Response[dcimv1.DeleteRoomResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) ListRackRows(ctx context.Context, req *connect.Request[dcimv1.ListRackRowsRequest]) (*connect.Response[dcimv1.ListRackRowsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) GetRackRow(ctx context.Context, req *connect.Request[dcimv1.GetRackRowRequest]) (*connect.Response[dcimv1.GetRackRowResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) CreateRackRow(ctx context.Context, req *connect.Request[dcimv1.CreateRackRowRequest]) (*connect.Response[dcimv1.CreateRackRowResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) UpdateRackRow(ctx context.Context, req *connect.Request[dcimv1.UpdateRackRowRequest]) (*connect.Response[dcimv1.UpdateRackRowResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) DeleteRackRow(ctx context.Context, req *connect.Request[dcimv1.DeleteRackRowRequest]) (*connect.Response[dcimv1.DeleteRackRowResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}
