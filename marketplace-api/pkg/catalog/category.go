package catalog

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	catalogv1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/catalog/v1"
	marketplacev1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/marketplace/v1"
)

func (s *Server) ListCategories(
	ctx context.Context,
	_ *catalogv1.ListCategoriesRequest,
) (*catalogv1.ListCategoriesResponse, error) {
	rows, err := s.queries.CategoryList(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing categories: %w", err))
	}

	categories := make([]*marketplacev1.Category, 0, len(rows))
	for _, row := range rows {
		categories = append(categories, marketplacev1.Category_builder{
			Id:   row.ID.String(),
			Name: row.Name,
		}.Build())
	}

	return catalogv1.ListCategoriesResponse_builder{
		Categories: categories,
	}.Build(), nil
}
