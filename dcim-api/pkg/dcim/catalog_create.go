package dcim

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) CreateCatalogEntry(
	ctx context.Context,
	req *connect.Request[dcimv1.CreateCatalogEntryRequest],
) (*connect.Response[dcimv1.CreateCatalogEntryResponse], error) {
	params := db.DeviceCatalogCreateParams{
		Manufacturer: req.Msg.GetManufacturer(),
		Model:        req.Msg.GetModel(),
		Category:     assetCategoryToDB(req.Msg.GetCategory()),
		Specs:        specsToDB(req.Msg.GetSpecs()),
	}

	if req.Msg.GetPartNumber() != "" {
		params.PartNumber = pgtype.Text{String: req.Msg.GetPartNumber(), Valid: true}
	}

	if req.Msg.GetFormFactor() != "" {
		params.FormFactor = pgtype.Text{String: req.Msg.GetFormFactor(), Valid: true}
	}

	if req.Msg.HasRackUnits() {
		params.RackUnits = pgtype.Int4{Int32: req.Msg.GetRackUnits(), Valid: true}
	}

	if req.Msg.GetWeightKg() != 0 {
		params.WeightKg = float64ToNumeric(req.Msg.GetWeightKg())
	}

	if req.Msg.GetPowerDrawW() != 0 {
		params.PowerDrawW = float64ToNumeric(req.Msg.GetPowerDrawW())
	}

	id, err := s.queries.DeviceCatalogCreate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == dbconst.ConstraintDeviceCatalogsUqManufacturerModel {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("catalog entry with this manufacturer and model already exists"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create catalog entry: %w", err))
	}

	s.logger.InfoContext(ctx, "catalog entry created", "catalog_entry_id", id)

	return connect.NewResponse(dcimv1.CreateCatalogEntryResponse_builder{
		CatalogEntryId: id.String(),
	}.Build()), nil
}
