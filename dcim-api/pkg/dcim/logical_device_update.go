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

func (s *Server) UpdateDevice(
	ctx context.Context,
	req *connect.Request[dcimv1.UpdateDeviceRequest],
) (*connect.Response[emptypb.Empty], error) {
	deviceID := uuid.MustParse(req.Msg.GetId())

	params := db.LogicalDeviceUpdateParams{
		ID: deviceID,
	}

	if req.Msg.HasLabel() {
		params.Label = pgtype.Text{String: req.Msg.GetLabel(), Valid: true}
	}

	if req.Msg.HasRole() {
		params.Role = pgtype.Text{String: logicalDeviceRoleToDB(req.Msg.GetRole()), Valid: true}
	}

	if req.Msg.HasDeviceCatalogId() {
		params.DeviceCatalogID = pgtype.UUID{Bytes: uuid.MustParse(req.Msg.GetDeviceCatalogId()), Valid: true}
	}

	if req.Msg.HasRequirements() {
		params.Requirements = pgtype.Text{String: req.Msg.GetRequirements(), Valid: true}
	}

	if req.Msg.HasNotes() {
		params.Notes = pgtype.Text{String: req.Msg.GetNotes(), Valid: true}
	}

	rowsAffected, err := s.queries.LogicalDeviceUpdate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.ConstraintName {
			case dbconst.ConstraintLogicalDevicesUqDesignLabel:
				return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("device with this label already exists in this design"))
			case dbconst.ConstraintDcimLogicalDevicesFkCatalog:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("device catalog entry not found"))
			case dbconst.ConstraintLogicalDevicesCkRole:
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid device role"))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update device: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("device not found"))
	}

	s.logger.InfoContext(ctx, "device updated", "device_id", deviceID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
