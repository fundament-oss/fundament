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

func (s *Server) UpdatePhysicalConnection(
	ctx context.Context,
	req *connect.Request[dcimv1.UpdatePhysicalConnectionRequest],
) (*connect.Response[emptypb.Empty], error) {
	connID := uuid.MustParse(req.Msg.GetId())

	params := db.PhysicalConnectionUpdateParams{
		ID: connID,
	}

	if req.Msg.HasCableAssetId() {
		if v := req.Msg.GetCableAssetId(); v == "" {
			params.ClearCableAssetID = true
		} else {
			params.CableAssetID = pgtype.UUID{Bytes: uuid.MustParse(v), Valid: true}
		}
	}

	if req.Msg.HasLogicalConnectionId() {
		if v := req.Msg.GetLogicalConnectionId(); v == "" {
			params.ClearLogicalConnectionID = true
		} else {
			params.LogicalConnectionID = pgtype.UUID{Bytes: uuid.MustParse(v), Valid: true}
		}
	}

	// For the presentation attributes, an explicitly-set field clears the column
	// when it carries the "empty" sentinel (UNSPECIFIED enum / empty label) and
	// otherwise overwrites it. Leaving the field unset keeps the current value.
	if req.Msg.HasCableType() {
		if t := req.Msg.GetCableType(); t == dcimv1.CableType_CABLE_TYPE_UNSPECIFIED {
			params.ClearCableType = true
		} else {
			params.CableType = cableTypeToDB(t)
		}
	}

	if req.Msg.HasStatus() {
		if st := req.Msg.GetStatus(); st == dcimv1.CableStatus_CABLE_STATUS_UNSPECIFIED {
			params.ClearStatus = true
		} else {
			params.Status = cableStatusToDB(st)
		}
	}

	if req.Msg.HasColor() {
		if c := req.Msg.GetColor(); c == dcimv1.CableColor_CABLE_COLOR_UNSPECIFIED {
			params.ClearColor = true
		} else {
			params.Color = cableColorToDB(c)
		}
	}

	if req.Msg.HasLabel() {
		if v := req.Msg.GetLabel(); v == "" {
			params.ClearLabel = true
		} else {
			params.Label = pgtype.Text{String: v, Valid: true}
		}
	}

	rowsAffected, err := s.queries.PhysicalConnectionUpdate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.ConstraintName {
			case dbconst.ConstraintDcimPhysicalConnectionsFkCableAsset:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cable asset not found"))
			case dbconst.ConstraintDcimPhysicalConnectionsFkLogicalConnection:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("logical connection not found"))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update physical connection: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("physical connection not found"))
	}

	s.logger.InfoContext(ctx, "physical connection updated", "connection_id", connID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
