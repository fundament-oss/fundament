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
	req *dcimv1.CreateDeviceRequest,
) (*dcimv1.CreateDeviceResponse, error) {
	params := db.LogicalDeviceCreateParams{
		LogicalDesignID: uuid.MustParse(req.GetDesignId()),
		Label:           req.GetLabel(),
		Role:            logicalDeviceRoleToDB(req.GetRole()),
	}

	if req.HasDeviceCatalogId() {
		params.DeviceCatalogID = pgtype.UUID{Bytes: uuid.MustParse(req.GetDeviceCatalogId()), Valid: true}
	}

	if req.HasRequirements() {
		params.Requirements = pgtype.Text{String: req.GetRequirements(), Valid: true}
	}

	if req.HasNotes() {
		params.Notes = pgtype.Text{String: req.GetNotes(), Valid: true}
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

	return dcimv1.CreateDeviceResponse_builder{
		DeviceId: id.String(),
	}.Build(), nil
}
