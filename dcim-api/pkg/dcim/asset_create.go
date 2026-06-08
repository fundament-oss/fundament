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
	req *connect.Request[dcimv1.CreateAssetRequest],
) (*connect.Response[dcimv1.CreateAssetResponse], error) {
	params := db.AssetCreateParams{
		DeviceCatalogID: uuid.MustParse(req.Msg.GetDeviceCatalogId()),
		Status:          assetStatusToDB(req.Msg.GetStatus()),
	}

	if req.Msg.HasSerialNumber() {
		params.SerialNumber = pgtype.Text{String: req.Msg.GetSerialNumber(), Valid: true}
	}

	if req.Msg.HasAssetTag() {
		params.AssetTag = pgtype.Text{String: req.Msg.GetAssetTag(), Valid: true}
	}

	if req.Msg.HasPurchaseDate() {
		params.PurchaseDate = pgtype.Date{
			Time:  req.Msg.GetPurchaseDate().AsTime(),
			Valid: true,
		}
	}

	if req.Msg.HasPurchaseOrder() {
		params.PurchaseOrder = pgtype.Text{String: req.Msg.GetPurchaseOrder(), Valid: true}
	}

	if req.Msg.HasWarrantyExpiry() {
		params.WarrantyExpiry = pgtype.Date{
			Time:  req.Msg.GetWarrantyExpiry().AsTime(),
			Valid: true,
		}
	}

	if req.Msg.HasNotes() {
		params.Notes = pgtype.Text{String: req.Msg.GetNotes(), Valid: true}
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

	return connect.NewResponse(dcimv1.CreateAssetResponse_builder{
		AssetId: id.String(),
	}.Build()), nil
}
