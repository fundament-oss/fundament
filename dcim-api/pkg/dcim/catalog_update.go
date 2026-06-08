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
	req *connect.Request[dcimv1.UpdateCatalogEntryRequest],
) (*connect.Response[emptypb.Empty], error) {
	catalogID := uuid.MustParse(req.Msg.GetId())

	params := db.DeviceCatalogUpdateParams{
		ID: catalogID,
	}

	if req.Msg.HasManufacturer() {
		params.Manufacturer = pgtype.Text{String: req.Msg.GetManufacturer(), Valid: true}
	}

	if req.Msg.HasModel() {
		params.Model = pgtype.Text{String: req.Msg.GetModel(), Valid: true}
	}

	if req.Msg.HasPartNumber() {
		params.PartNumber = pgtype.Text{String: req.Msg.GetPartNumber(), Valid: true}
	}

	if req.Msg.GetCategory() != dcimv1.AssetCategory_ASSET_CATEGORY_UNSPECIFIED {
		params.Category = pgtype.Text{String: assetCategoryToDB(req.Msg.GetCategory()), Valid: true}
	}

	if req.Msg.HasFormFactor() {
		params.FormFactor = pgtype.Text{String: req.Msg.GetFormFactor(), Valid: true}
	}

	if req.Msg.HasRackUnits() {
		params.RackUnits = pgtype.Int4{Int32: req.Msg.GetRackUnits(), Valid: true}
	}

	if req.Msg.HasWeightKg() {
		params.WeightKg = float64ToNumeric(req.Msg.GetWeightKg())
	}

	if req.Msg.HasPowerDrawW() {
		params.PowerDrawW = float64ToNumeric(req.Msg.GetPowerDrawW())
	}

	if len(req.Msg.GetSpecs()) > 0 {
		params.Specs = specsToDB(req.Msg.GetSpecs())
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

	return connect.NewResponse(&emptypb.Empty{}), nil
}
