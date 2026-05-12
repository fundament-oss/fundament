package dcim

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) GetCatalogEntry(
	ctx context.Context,
	req *connect.Request[dcimv1.GetCatalogEntryRequest],
) (*connect.Response[dcimv1.GetCatalogEntryResponse], error) {
	catalogID := uuid.MustParse(req.Msg.GetId())

	row, err := s.queries.DeviceCatalogGetByID(ctx, db.DeviceCatalogGetByIDParams{ID: catalogID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("catalog entry not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get catalog entry: %w", err))
	}

	return connect.NewResponse(dcimv1.GetCatalogEntryResponse_builder{
		Entry: catalogFromGetRow(&row),
	}.Build()), nil
}
