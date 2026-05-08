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
	req *connect.Request[dcimv1.CreatePortDefinitionRequest],
) (*connect.Response[dcimv1.CreatePortDefinitionResponse], error) {
	params := db.PortDefinitionCreateParams{
		DeviceCatalogID: uuid.MustParse(req.Msg.GetDeviceCatalogId()),
		Name:            req.Msg.GetName(),
		PortType:        portTypeToDB(req.Msg.GetPortType()),
		Direction:       portDirectionToDB(req.Msg.GetDirection()),
		Ordinal:         req.Msg.GetOrdinal(),
	}

	if req.Msg.GetMediaType() != "" {
		params.MediaType = pgtype.Text{String: req.Msg.GetMediaType(), Valid: true}
	}

	if req.Msg.HasSpeed() {
		params.Speed = pgtype.Text{String: req.Msg.GetSpeed(), Valid: true}
	}

	if req.Msg.HasMaxPowerW() {
		params.MaxPowerW = float64ToNumeric(req.Msg.GetMaxPowerW())
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

	return connect.NewResponse(dcimv1.CreatePortDefinitionResponse_builder{
		PortDefinitionId: id.String(),
	}.Build()), nil
}
