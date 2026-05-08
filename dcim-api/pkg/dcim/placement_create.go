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

func (s *Server) CreatePlacement(
	ctx context.Context,
	req *connect.Request[dcimv1.CreatePlacementRequest],
) (*connect.Response[dcimv1.CreatePlacementResponse], error) {
	params := db.PlacementCreateParams{
		AssetID: uuid.MustParse(req.Msg.GetAssetId()),
	}

	switch loc := req.Msg.WhichLocation(); loc {
	case dcimv1.CreatePlacementRequest_Rack_case:
		rack := req.Msg.GetRack()
		params.RackID = pgtype.UUID{Bytes: uuid.MustParse(rack.GetRackId()), Valid: true}
		params.StartUnit = pgtype.Int4{Int32: rack.GetRackUnitStart(), Valid: true}
		params.SlotType = pgtype.Text{String: rackSlotTypeToDB(rack.GetRackSlotType()), Valid: true}
	case dcimv1.CreatePlacementRequest_SubComponent_case:
		sub := req.Msg.GetSubComponent()
		params.ParentPlacementID = pgtype.UUID{Bytes: uuid.MustParse(sub.GetParentPlacementId()), Valid: true}
		params.PortDefinitionID = pgtype.UUID{Bytes: uuid.MustParse(sub.GetParentPortName()), Valid: true}
	default:
		panic("unhandled placement location case")
	}

	if req.Msg.HasLogicalDeviceId() {
		params.LogicalDeviceID = pgtype.UUID{Bytes: uuid.MustParse(req.Msg.GetLogicalDeviceId()), Valid: true}
	}

	if req.Msg.GetNotes() != "" {
		params.Notes = pgtype.Text{String: req.Msg.GetNotes(), Valid: true}
	}

	id, err := s.queries.PlacementCreate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.ConstraintName {
			case dbconst.ConstraintDcimPlacementsFkAsset:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("asset not found"))
			case dbconst.ConstraintDcimPlacementsFkRack:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("rack not found"))
			case dbconst.ConstraintDcimPlacementsFkParent:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("parent placement not found"))
			case dbconst.ConstraintDcimPlacementsFkPortDefinition:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("port definition not found"))
			case dbconst.ConstraintPlacementsCkExclusiveArc:
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("placement must have either rack or sub-component location, not both"))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create placement: %w", err))
	}

	s.logger.InfoContext(ctx, "placement created", "placement_id", id)

	return connect.NewResponse(dcimv1.CreatePlacementResponse_builder{
		PlacementId: id.String(),
	}.Build()), nil
}
