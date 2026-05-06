package dcim

import (
	"context"

	"connectrpc.com/connect"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) ListAssets(ctx context.Context, req *connect.Request[dcimv1.ListAssetsRequest]) (*connect.Response[dcimv1.ListAssetsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) GetAsset(ctx context.Context, req *connect.Request[dcimv1.GetAssetRequest]) (*connect.Response[dcimv1.GetAssetResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) CreateAsset(ctx context.Context, req *connect.Request[dcimv1.CreateAssetRequest]) (*connect.Response[dcimv1.CreateAssetResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) UpdateAsset(ctx context.Context, req *connect.Request[dcimv1.UpdateAssetRequest]) (*connect.Response[dcimv1.UpdateAssetResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) DeleteAsset(ctx context.Context, req *connect.Request[dcimv1.DeleteAssetRequest]) (*connect.Response[dcimv1.DeleteAssetResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) GetAssetEvents(ctx context.Context, req *connect.Request[dcimv1.GetAssetEventsRequest]) (*connect.Response[dcimv1.GetAssetEventsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) GetAssetStats(ctx context.Context, req *connect.Request[dcimv1.GetAssetStatsRequest]) (*connect.Response[dcimv1.GetAssetStatsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}
