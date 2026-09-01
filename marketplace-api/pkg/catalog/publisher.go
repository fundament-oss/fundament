package catalog

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	catalogv1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/catalog/v1"
	marketplacev1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/marketplace/v1"
)

// RLS restricts tenant.organizations to publishers with a live public listing,
// so this needs no filtering of its own.
func (s *Server) ListPublishers(
	ctx context.Context,
	_ *catalogv1.ListPublishersRequest,
) (*catalogv1.ListPublishersResponse, error) {
	rows, err := s.queries.PublisherList(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing publishers: %w", err))
	}

	publishers := make([]*marketplacev1.Publisher, 0, len(rows))
	for _, row := range rows {
		publishers = append(publishers, marketplacev1.Publisher_builder{
			Id:          row.ID.String(),
			Name:        row.Name,
			DisplayName: row.Alias,
		}.Build())
	}

	return catalogv1.ListPublishersResponse_builder{
		Publishers: publishers,
	}.Build(), nil
}
