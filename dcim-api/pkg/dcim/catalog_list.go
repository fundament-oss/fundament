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

func (s *Server) ListCatalog(
	ctx context.Context,
	req *connect.Request[dcimv1.ListCatalogRequest],
) (*connect.Response[dcimv1.ListCatalogResponse], error) {
	params := db.DeviceCatalogListParams{}

	if req.Msg.HasCategoryFilter() {
		params.Category = pgtype.Text{String: assetCategoryToDB(req.Msg.GetCategoryFilter()), Valid: true}
	}

	if req.Msg.HasSearch() {
		params.Search = pgtype.Text{String: req.Msg.GetSearch(), Valid: true}
	}

	rows, err := s.queries.DeviceCatalogList(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list catalog entries: %w", err))
	}

	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}

	counts, err := s.queries.DeviceCatalogAssetCounts(ctx, db.DeviceCatalogAssetCountsParams{Ids: ids})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get asset counts: %w", err))
	}

	countMap := make(map[uuid.UUID]*db.DeviceCatalogAssetCountsRow, len(counts))
	for i := range counts {
		countMap[counts[i].DeviceCatalogID] = &counts[i]
	}

	entries := make([]*dcimv1.ListCatalogResponse_CatalogSummary, 0, len(rows))
	for _, row := range rows {
		summary := dcimv1.ListCatalogResponse_CatalogSummary_builder{
			Entry: catalogFromListRow(&row),
		}

		if c, ok := countMap[row.ID]; ok {
			summary.Total = c.Total
			summary.Deployed = c.Deployed
			summary.Available = c.Available
			summary.NeedsRepair = c.NeedsRepair
		}

		entries = append(entries, summary.Build())
	}

	return connect.NewResponse(dcimv1.ListCatalogResponse_builder{
		Entries: entries,
	}.Build()), nil
}
