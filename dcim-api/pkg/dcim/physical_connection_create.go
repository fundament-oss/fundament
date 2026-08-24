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

func (s *Server) CreatePhysicalConnection(
	ctx context.Context,
	req *dcimv1.CreatePhysicalConnectionRequest,
) (*dcimv1.CreatePhysicalConnectionResponse, error) {
	params := db.PhysicalConnectionCreateParams{
		APlacementID:      uuid.MustParse(req.GetSourcePlacementId()),
		APortDefinitionID: uuid.MustParse(req.GetSourcePortDefinitionId()),
		BPlacementID:      uuid.MustParse(req.GetTargetPlacementId()),
		BPortDefinitionID: uuid.MustParse(req.GetTargetPortDefinitionId()),
		CableType:         cableTypeToDB(req.GetCableType()),
		Status:            cableStatusToDB(req.GetStatus()),
		Color:             cableColorToDB(req.GetColor()),
		Label:             pgtype.Text{String: req.GetLabel(), Valid: req.GetLabel() != ""},
	}

	if req.HasCableAssetId() {
		params.CableAssetID = pgtype.UUID{Bytes: uuid.MustParse(req.GetCableAssetId()), Valid: true}
	}

	if req.HasLogicalConnectionId() {
		params.LogicalConnectionID = pgtype.UUID{Bytes: uuid.MustParse(req.GetLogicalConnectionId()), Valid: true}
	}

	id, err := s.queries.PhysicalConnectionCreate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.ConstraintName {
			case dbconst.ConstraintDcimPhysicalConnectionsFkAPlacement:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("source placement not found"))
			case dbconst.ConstraintDcimPhysicalConnectionsFkBPlacement:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("target placement not found"))
			case dbconst.ConstraintDcimPhysicalConnectionsFkAPort:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("source port definition not found"))
			case dbconst.ConstraintDcimPhysicalConnectionsFkBPort:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("target port definition not found"))
			case dbconst.ConstraintDcimPhysicalConnectionsFkCableAsset:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cable asset not found"))
			case dbconst.ConstraintDcimPhysicalConnectionsFkLogicalConnection:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("logical connection not found"))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create physical connection: %w", err))
	}

	s.logger.InfoContext(ctx, "physical connection created", "connection_id", id)

	return dcimv1.CreatePhysicalConnectionResponse_builder{
		ConnectionId: id.String(),
	}.Build(), nil
}
