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
	req *connect.Request[dcimv1.UpdatePortDefinitionRequest],
) (*connect.Response[emptypb.Empty], error) {
	portDefID := uuid.MustParse(req.Msg.GetId())

	params := db.PortDefinitionUpdateParams{
		ID: portDefID,
	}

	if req.Msg.HasName() {
		params.Name = pgtype.Text{String: req.Msg.GetName(), Valid: true}
	}

	if req.Msg.HasPortType() {
		params.PortType = pgtype.Text{String: portTypeToDB(req.Msg.GetPortType()), Valid: true}
	}

	if req.Msg.HasMediaType() {
		params.MediaType = pgtype.Text{String: req.Msg.GetMediaType(), Valid: true}
	}

	if req.Msg.HasSpeed() {
		params.Speed = pgtype.Text{String: req.Msg.GetSpeed(), Valid: true}
	}

	if req.Msg.HasMaxPowerW() {
		params.MaxPowerW = float64ToNumeric(req.Msg.GetMaxPowerW())
	}

	if req.Msg.HasDirection() {
		params.Direction = pgtype.Text{String: portDirectionToDB(req.Msg.GetDirection()), Valid: true}
	}

	if req.Msg.HasOrdinal() {
		params.Ordinal = pgtype.Int4{Int32: req.Msg.GetOrdinal(), Valid: true}
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

	return connect.NewResponse(&emptypb.Empty{}), nil
}
