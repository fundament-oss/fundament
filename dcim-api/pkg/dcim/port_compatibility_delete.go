package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/emptypb"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) DeletePortCompatibility(
	ctx context.Context,
	req *dcimv1.DeletePortCompatibilityRequest,
) (*emptypb.Empty, error) {
	portDefID := uuid.MustParse(req.GetPortDefinitionId())
	compatCatalogID := uuid.MustParse(req.GetCompatibleCatalogId())

	rowsAffected, err := s.queries.PortCompatibilityDelete(ctx, db.PortCompatibilityDeleteParams{
		PortDefinitionID: portDefID,
		CompatibleCatalogID: pgtype.UUID{
			Bytes: compatCatalogID,
			Valid: true,
		},
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete port compatibility: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("port compatibility not found"))
	}

	s.logger.InfoContext(ctx, "port compatibility deleted", "port_definition_id", portDefID, "compatible_catalog_id", compatCatalogID)

	return &emptypb.Empty{}, nil
}
