package dcim

import (
	"context"

	"connectrpc.com/connect"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Device catalog entries

func (s *Server) ListCatalog(ctx context.Context, req *connect.Request[dcimv1.ListCatalogRequest]) (*connect.Response[dcimv1.ListCatalogResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) GetCatalogEntry(ctx context.Context, req *connect.Request[dcimv1.GetCatalogEntryRequest]) (*connect.Response[dcimv1.GetCatalogEntryResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) CreateCatalogEntry(ctx context.Context, req *connect.Request[dcimv1.CreateCatalogEntryRequest]) (*connect.Response[dcimv1.CreateCatalogEntryResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) UpdateCatalogEntry(ctx context.Context, req *connect.Request[dcimv1.UpdateCatalogEntryRequest]) (*connect.Response[emptypb.Empty], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) DeleteCatalogEntry(ctx context.Context, req *connect.Request[dcimv1.DeleteCatalogEntryRequest]) (*connect.Response[emptypb.Empty], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) ListAssetsByCatalogEntry(ctx context.Context, req *connect.Request[dcimv1.ListAssetsByCatalogEntryRequest]) (*connect.Response[dcimv1.ListAssetsByCatalogEntryResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

// Port definitions

func (s *Server) ListPortDefinitions(ctx context.Context, req *connect.Request[dcimv1.ListPortDefinitionsRequest]) (*connect.Response[dcimv1.ListPortDefinitionsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) GetPortDefinition(ctx context.Context, req *connect.Request[dcimv1.GetPortDefinitionRequest]) (*connect.Response[dcimv1.GetPortDefinitionResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) CreatePortDefinition(ctx context.Context, req *connect.Request[dcimv1.CreatePortDefinitionRequest]) (*connect.Response[dcimv1.CreatePortDefinitionResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) UpdatePortDefinition(ctx context.Context, req *connect.Request[dcimv1.UpdatePortDefinitionRequest]) (*connect.Response[emptypb.Empty], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) DeletePortDefinition(ctx context.Context, req *connect.Request[dcimv1.DeletePortDefinitionRequest]) (*connect.Response[emptypb.Empty], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

// Port compatibilities

func (s *Server) ListPortCompatibilities(ctx context.Context, req *connect.Request[dcimv1.ListPortCompatibilitiesRequest]) (*connect.Response[dcimv1.ListPortCompatibilitiesResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) CreatePortCompatibility(ctx context.Context, req *connect.Request[dcimv1.CreatePortCompatibilityRequest]) (*connect.Response[emptypb.Empty], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) DeletePortCompatibility(ctx context.Context, req *connect.Request[dcimv1.DeletePortCompatibilityRequest]) (*connect.Response[emptypb.Empty], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}
