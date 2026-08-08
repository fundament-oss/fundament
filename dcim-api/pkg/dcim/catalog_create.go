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
	req *dcimv1.CreateCatalogEntryRequest,
) (*dcimv1.CreateCatalogEntryResponse, error) {
	params := db.DeviceCatalogCreateParams{
		Manufacturer: req.GetManufacturer(),
		Model:        req.GetModel(),
		PartNumber:   pgtype.Text{String: req.GetPartNumber(), Valid: true},
		Category:     assetCategoryToDB(req.GetCategory()),
		Specs:        specsToDB(req.GetSpecs()),
	}

	if req.HasFormFactor() {
		params.FormFactor = pgtype.Text{String: req.GetFormFactor(), Valid: true}
	}

	if req.HasRackUnits() {
		params.RackUnits = pgtype.Int4{Int32: req.GetRackUnits(), Valid: true}
	}

	if req.GetWeightKg() != 0 {
		params.WeightKg = float64ToNumeric(req.GetWeightKg())
	}

	if req.GetPowerDrawW() != 0 {
		params.PowerDrawW = float64ToNumeric(req.GetPowerDrawW())
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

	return dcimv1.CreateCatalogEntryResponse_builder{
		CatalogEntryId: id.String(),
	}.Build(), nil
}
