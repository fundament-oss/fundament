package dcim

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) CreatePortCompatibility(
	ctx context.Context,
	req *connect.Request[dcimv1.CreatePortCompatibilityRequest],
) (*connect.Response[emptypb.Empty], error) {
	portDefID := uuid.MustParse(req.Msg.GetPortDefinitionId())
	compatCatalogID := uuid.MustParse(req.Msg.GetCompatibleCatalogId())

	_, err := s.queries.PortCompatibilityCreate(ctx, db.PortCompatibilityCreateParams{
		PortDefinitionID:    portDefID,
		CompatibleCatalogID: compatCatalogID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("compatible catalog entry not found"))
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.ConstraintName {
			case dbconst.ConstraintDcimPortCompatibilitiesFkPortDefinition:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("port definition not found"))
			case dbconst.ConstraintDcimPortCompatibilitiesFkCatalog:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("compatible catalog entry not found"))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create port compatibility: %w", err))
	}

	s.logger.InfoContext(ctx, "port compatibility created", "port_definition_id", portDefID, "compatible_catalog_id", compatCatalogID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
