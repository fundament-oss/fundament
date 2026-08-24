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

func (s *Server) UpdateCatalogEntry(
	ctx context.Context,
	req *dcimv1.UpdateCatalogEntryRequest,
) (*emptypb.Empty, error) {
	catalogID := uuid.MustParse(req.GetId())

	params := db.DeviceCatalogUpdateParams{
		ID: catalogID,
	}

	if req.HasManufacturer() {
		params.Manufacturer = pgtype.Text{String: req.GetManufacturer(), Valid: true}
	}

	if req.HasModel() {
		params.Model = pgtype.Text{String: req.GetModel(), Valid: true}
	}

	if req.HasPartNumber() {
		params.PartNumber = pgtype.Text{String: req.GetPartNumber(), Valid: true}
	}

	if req.GetCategory() != dcimv1.AssetCategory_ASSET_CATEGORY_UNSPECIFIED {
		params.Category = pgtype.Text{String: assetCategoryToDB(req.GetCategory()), Valid: true}
	}

	if req.HasFormFactor() {
		params.FormFactor = pgtype.Text{String: req.GetFormFactor(), Valid: true}
	}

	if req.HasRackUnits() {
		params.RackUnits = pgtype.Int4{Int32: req.GetRackUnits(), Valid: true}
	}

	if req.HasWeightKg() {
		params.WeightKg = float64ToNumeric(req.GetWeightKg())
	}

	if req.HasPowerDrawW() {
		params.PowerDrawW = float64ToNumeric(req.GetPowerDrawW())
	}

	if len(req.GetSpecs()) > 0 {
		params.Specs = specsToDB(req.GetSpecs())
	}

	rowsAffected, err := s.queries.DeviceCatalogUpdate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == dbconst.ConstraintDeviceCatalogsUqManufacturerModel {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("catalog entry with this manufacturer and model already exists"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update catalog entry: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("catalog entry not found"))
	}

	s.logger.InfoContext(ctx, "catalog entry updated", "catalog_entry_id", catalogID)

	return &emptypb.Empty{}, nil
}
