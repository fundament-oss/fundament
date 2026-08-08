package dcim

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) UpdatePortDefinition(
	ctx context.Context,
	req *dcimv1.UpdatePortDefinitionRequest,
) (*emptypb.Empty, error) {
	portDefID := uuid.MustParse(req.GetId())

	params := db.PortDefinitionUpdateParams{
		ID: portDefID,
	}

	if req.HasName() {
		params.Name = pgtype.Text{String: req.GetName(), Valid: true}
	}

	if req.HasPortType() {
		params.PortType = pgtype.Text{String: portTypeToDB(req.GetPortType()), Valid: true}
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

	if req.HasDirection() {
		params.Direction = pgtype.Text{String: portDirectionToDB(req.GetDirection()), Valid: true}
	}

	if req.HasOrdinal() {
		params.Ordinal = pgtype.Int4{Int32: req.GetOrdinal(), Valid: true}
	}

	rowsAffected, err := s.queries.PortDefinitionUpdate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == dbconst.ConstraintPortDefinitionsUqCatalogName {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("port definition with this name already exists for this catalog entry"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update port definition: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("port definition not found"))
	}

	s.logger.InfoContext(ctx, "port definition updated", "port_definition_id", portDefID)

	return &emptypb.Empty{}, nil
}
