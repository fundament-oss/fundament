package dcim

import (
	"context"

	"connectrpc.com/connect"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *Server) ListRackRows(ctx context.Context, req *connect.Request[dcimv1.ListRackRowsRequest]) (*connect.Response[dcimv1.ListRackRowsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) GetRackRow(ctx context.Context, req *connect.Request[dcimv1.GetRackRowRequest]) (*connect.Response[dcimv1.GetRackRowResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) CreateRackRow(ctx context.Context, req *connect.Request[dcimv1.CreateRackRowRequest]) (*connect.Response[dcimv1.CreateRackRowResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) UpdateRackRow(ctx context.Context, req *connect.Request[dcimv1.UpdateRackRowRequest]) (*connect.Response[emptypb.Empty], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) DeleteRackRow(ctx context.Context, req *connect.Request[dcimv1.DeleteRackRowRequest]) (*connect.Response[emptypb.Empty], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}
