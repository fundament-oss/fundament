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

func (s *Server) UpdateAsset(
	ctx context.Context,
	req *dcimv1.UpdateAssetRequest,
) (*emptypb.Empty, error) {
	assetID := uuid.MustParse(req.GetId())

	params := db.AssetUpdateParams{
		ID: assetID,
	}

	if req.HasStatus() {
		params.Status = pgtype.Text{String: assetStatusToDB(req.GetStatus()), Valid: true}
	}

	if req.HasSerialNumber() {
		params.SerialNumber = pgtype.Text{String: req.GetSerialNumber(), Valid: true}
	}

	if req.HasAssetTag() {
		params.AssetTag = pgtype.Text{String: req.GetAssetTag(), Valid: true}
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

	rowsAffected, err := s.queries.AssetUpdate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.ConstraintName {
			case dbconst.ConstraintAssetsUqSerialNumber:
				return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("asset with this serial number already exists"))
			case dbconst.ConstraintAssetsUqAssetTag:
				return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("asset with this asset tag already exists"))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update asset: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("asset not found"))
	}

	s.logger.InfoContext(ctx, "asset updated", "asset_id", assetID)

	return &emptypb.Empty{}, nil
}
