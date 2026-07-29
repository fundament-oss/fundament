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
	req *dcimv1.UpdatePhysicalConnectionRequest,
) (*emptypb.Empty, error) {
	connID := uuid.MustParse(req.GetId())

	params := db.PhysicalConnectionUpdateParams{
		ID: connID,
	}

	if req.HasCableAssetId() {
		if v := req.GetCableAssetId(); v == "" {
			params.ClearCableAssetID = true
		} else {
			params.CableAssetID = pgtype.UUID{Bytes: uuid.MustParse(v), Valid: true}
		}
	}

	if req.HasLogicalConnectionId() {
		if v := req.GetLogicalConnectionId(); v == "" {
			params.ClearLogicalConnectionID = true
		} else {
			params.LogicalConnectionID = pgtype.UUID{Bytes: uuid.MustParse(v), Valid: true}
		}
	}

	// For the presentation attributes, an explicitly-set field clears the column
	// when it carries the "empty" sentinel (UNSPECIFIED enum / empty label) and
	// otherwise overwrites it. Leaving the field unset keeps the current value.
	if req.HasCableType() {
		if t := req.GetCableType(); t == dcimv1.CableType_CABLE_TYPE_UNSPECIFIED {
			params.ClearCableType = true
		} else {
			params.CableType = cableTypeToDB(t)
		}
	}

	if req.HasStatus() {
		if st := req.GetStatus(); st == dcimv1.CableStatus_CABLE_STATUS_UNSPECIFIED {
			params.ClearStatus = true
		} else {
			params.Status = cableStatusToDB(st)
		}
	}

	if req.HasColor() {
		if c := req.GetColor(); c == dcimv1.CableColor_CABLE_COLOR_UNSPECIFIED {
			params.ClearColor = true
		} else {
			params.Color = cableColorToDB(c)
		}
	}

	if req.HasLabel() {
		if v := req.GetLabel(); v == "" {
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

	return &emptypb.Empty{}, nil
}
