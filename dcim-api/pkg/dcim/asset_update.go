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
	req *connect.Request[dcimv1.UpdateAssetRequest],
) (*connect.Response[emptypb.Empty], error) {
	assetID := uuid.MustParse(req.Msg.GetId())

	params := db.AssetUpdateParams{
		ID: assetID,
	}

	if req.Msg.HasStatus() {
		params.Status = pgtype.Text{String: assetStatusToDB(req.Msg.GetStatus()), Valid: true}
	}

	if req.Msg.HasSerialNumber() {
		params.SerialNumber = pgtype.Text{String: req.Msg.GetSerialNumber(), Valid: true}
	}

	if req.Msg.HasAssetTag() {
		params.AssetTag = pgtype.Text{String: req.Msg.GetAssetTag(), Valid: true}
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

	return connect.NewResponse(&emptypb.Empty{}), nil
}
