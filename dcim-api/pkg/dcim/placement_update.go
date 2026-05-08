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

func (s *Server) UpdatePlacement(
	ctx context.Context,
	req *connect.Request[dcimv1.UpdatePlacementRequest],
) (*connect.Response[emptypb.Empty], error) {
	placementID := uuid.MustParse(req.Msg.GetId())

	params := db.PlacementUpdateParams{
		ID: placementID,
	}

	switch loc := req.Msg.WhichLocation(); loc {
	case dcimv1.UpdatePlacementRequest_Rack_case:
		rack := req.Msg.GetRack()
		params.RackID = pgtype.UUID{Bytes: uuid.MustParse(rack.GetRackId()), Valid: true}
		params.StartUnit = pgtype.Int4{Int32: rack.GetRackUnitStart(), Valid: true}
		params.SlotType = pgtype.Text{String: rackSlotTypeToDB(rack.GetRackSlotType()), Valid: true}
	case dcimv1.UpdatePlacementRequest_SubComponent_case:
		sub := req.Msg.GetSubComponent()
		params.ParentPlacementID = pgtype.UUID{Bytes: uuid.MustParse(sub.GetParentPlacementId()), Valid: true}
		params.PortDefinitionID = pgtype.UUID{Bytes: uuid.MustParse(sub.GetParentPortName()), Valid: true}
	case dcimv1.UpdatePlacementRequest_Location_not_set_case:
		// location not being updated
	default:
		panic("unhandled placement location case")
	}

	if req.Msg.HasLogicalDeviceId() {
		params.LogicalDeviceID = pgtype.UUID{Bytes: uuid.MustParse(req.Msg.GetLogicalDeviceId()), Valid: true}
	}

	if req.Msg.HasNotes() {
		params.Notes = pgtype.Text{String: req.Msg.GetNotes(), Valid: true}
	}

	rowsAffected, err := s.queries.PlacementUpdate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.ConstraintName {
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
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update placement: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("placement not found"))
	}

	s.logger.InfoContext(ctx, "placement updated", "placement_id", placementID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
