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

func (s *Server) DeleteCatalogEntry(
	ctx context.Context,
	req *connect.Request[dcimv1.DeleteCatalogEntryRequest],
) (*connect.Response[emptypb.Empty], error) {
	catalogID := uuid.MustParse(req.Msg.GetId())

	rowsAffected, err := s.queries.DeviceCatalogDelete(ctx, db.DeviceCatalogDeleteParams{ID: catalogID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete catalog entry: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("catalog entry not found"))
	}

	s.logger.InfoContext(ctx, "catalog entry deleted", "catalog_entry_id", catalogID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
