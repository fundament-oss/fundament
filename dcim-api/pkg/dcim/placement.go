package dcim

import (
	"context"

	"connectrpc.com/connect"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *Server) CreatePlacement(ctx context.Context, req *connect.Request[dcimv1.CreatePlacementRequest]) (*connect.Response[dcimv1.CreatePlacementResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) GetPlacement(ctx context.Context, req *connect.Request[dcimv1.GetPlacementRequest]) (*connect.Response[dcimv1.GetPlacementResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) UpdatePlacement(ctx context.Context, req *connect.Request[dcimv1.UpdatePlacementRequest]) (*connect.Response[emptypb.Empty], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) DeletePlacement(ctx context.Context, req *connect.Request[dcimv1.DeletePlacementRequest]) (*connect.Response[emptypb.Empty], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) ListPlacementsByRack(ctx context.Context, req *connect.Request[dcimv1.ListPlacementsByRackRequest]) (*connect.Response[dcimv1.ListPlacementsByRackResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) ListChildPlacements(ctx context.Context, req *connect.Request[dcimv1.ListChildPlacementsRequest]) (*connect.Response[dcimv1.ListChildPlacementsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}
