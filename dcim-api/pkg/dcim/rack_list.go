package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) ListRacks(
	ctx context.Context,
	req *dcimv1.ListRacksRequest,
) (*dcimv1.ListRacksResponse, error) {
	params := db.RackListParams{}

	if req.HasRowId() {
		rowID := uuid.MustParse(req.GetRowId())
		params.RackRowID = pgtype.UUID{Bytes: rowID, Valid: true}
	}
	if req.HasSiteId() {
		siteID := uuid.MustParse(req.GetSiteId())
		params.SiteID = pgtype.UUID{Bytes: siteID, Valid: true}
	}

	rows, err := s.queries.RackList(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list racks: %w", err))
	}

	// TODO: populate utilization metrics (used_units, free_units, power_draw_w,
	// device_count, utilization_pct) once device/placement tables are implemented.
	racks := make([]*dcimv1.ListRacksResponse_RackSummary, 0, len(rows))
	for _, row := range rows {
		racks = append(racks, dcimv1.ListRacksResponse_RackSummary_builder{
			Rack: rackFromListRow(&row),
		}.Build())
	}

	return dcimv1.ListRacksResponse_builder{
		Racks: racks,
	}.Build(), nil
}
