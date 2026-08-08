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

func (s *Server) CreateAsset(
	ctx context.Context,
	req *dcimv1.CreateAssetRequest,
) (*dcimv1.CreateAssetResponse, error) {
	params := db.AssetCreateParams{
		DeviceCatalogID: uuid.MustParse(req.GetDeviceCatalogId()),
		Status:          assetStatusToDB(req.GetStatus()),
	}

	if req.HasSerialNumber() {
		params.SerialNumber = pgtype.Text{String: req.GetSerialNumber(), Valid: true}
	}

	if req.HasAssetTag() {
		params.AssetTag = pgtype.Text{String: req.GetAssetTag(), Valid: true}
	}

	if req.HasPurchaseDate() {
		params.PurchaseDate = pgtype.Date{
			Time:  req.GetPurchaseDate().AsTime(),
			Valid: true,
		}
	}

	if req.HasPurchaseOrder() {
		params.PurchaseOrder = pgtype.Text{String: req.GetPurchaseOrder(), Valid: true}
	}

	if req.HasWarrantyExpiry() {
		params.WarrantyExpiry = pgtype.Date{
			Time:  req.GetWarrantyExpiry().AsTime(),
			Valid: true,
		}
	}

	if req.HasNotes() {
		params.Notes = pgtype.Text{String: req.GetNotes(), Valid: true}
	}

	id, err := s.queries.AssetCreate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.ConstraintName {
			case dbconst.ConstraintDcimAssetsFkDeviceCatalog:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("device catalog entry not found"))
			case dbconst.ConstraintAssetsUqSerialNumber:
				return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("asset with this serial number already exists"))
			case dbconst.ConstraintAssetsUqAssetTag:
				return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("asset with this asset tag already exists"))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create asset: %w", err))
	}

	s.logger.InfoContext(ctx, "asset created", "asset_id", id)

	return dcimv1.CreateAssetResponse_builder{
		AssetId: id.String(),
	}.Build(), nil
}
