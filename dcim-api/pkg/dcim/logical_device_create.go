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

func (s *Server) CreateDevice(
	ctx context.Context,
	req *connect.Request[dcimv1.CreateDeviceRequest],
) (*connect.Response[dcimv1.CreateDeviceResponse], error) {
	params := db.LogicalDeviceCreateParams{
		LogicalDesignID: uuid.MustParse(req.Msg.GetDesignId()),
		Label:           req.Msg.GetLabel(),
		Role:            logicalDeviceRoleToDB(req.Msg.GetRole()),
	}

	if req.Msg.HasDeviceCatalogId() {
		params.DeviceCatalogID = pgtype.UUID{Bytes: uuid.MustParse(req.Msg.GetDeviceCatalogId()), Valid: true}
	}

	if req.Msg.HasRequirements() {
		params.Requirements = pgtype.Text{String: req.Msg.GetRequirements(), Valid: true}
	}

	if req.Msg.GetNotes() != "" {
		params.Notes = pgtype.Text{String: req.Msg.GetNotes(), Valid: true}
	}

	id, err := s.queries.LogicalDeviceCreate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.ConstraintName {
			case dbconst.ConstraintLogicalDevicesUqDesignLabel:
				return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("device with this label already exists in this design"))
			case dbconst.ConstraintDcimLogicalDevicesFkDesign:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("design not found"))
			case dbconst.ConstraintDcimLogicalDevicesFkCatalog:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("device catalog entry not found"))
			case dbconst.ConstraintLogicalDevicesCkRole:
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid device role"))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create device: %w", err))
	}

	s.logger.InfoContext(ctx, "device created", "device_id", id)

	return connect.NewResponse(dcimv1.CreateDeviceResponse_builder{
		DeviceId: id.String(),
	}.Build()), nil
}
