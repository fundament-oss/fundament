package dcim

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) CreatePortDefinition(
	ctx context.Context,
	req *dcimv1.CreatePortDefinitionRequest,
) (*dcimv1.CreatePortDefinitionResponse, error) {
	params := db.PortDefinitionCreateParams{
		DeviceCatalogID: uuid.MustParse(req.GetDeviceCatalogId()),
		Name:            req.GetName(),
		PortType:        portTypeToDB(req.GetPortType()),
		Direction:       portDirectionToDB(req.GetDirection()),
		Ordinal:         req.GetOrdinal(),
	}

	if req.HasMediaType() {
		params.MediaType = pgtype.Text{String: req.GetMediaType(), Valid: true}
	}

	if req.HasSpeed() {
		params.Speed = pgtype.Text{String: req.GetSpeed(), Valid: true}
	}

	if req.HasMaxPowerW() {
		params.MaxPowerW = float64ToNumeric(req.GetMaxPowerW())
	}

	id, err := s.queries.PortDefinitionCreate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.ConstraintName {
			case dbconst.ConstraintPortDefinitionsUqCatalogName:
				return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("port definition with this name already exists for this catalog entry"))
			case dbconst.ConstraintDcimPortDefinitionsFkDeviceCatalog:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("device catalog entry not found"))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create port definition: %w", err))
	}

	s.logger.InfoContext(ctx, "port definition created", "port_definition_id", id)

	return dcimv1.CreatePortDefinitionResponse_builder{
		PortDefinitionId: id.String(),
	}.Build(), nil
}
