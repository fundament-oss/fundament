package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"golang.org/x/sync/errgroup"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/organization-api/pkg/organization/adapter"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *OrganizationServer) ListPlugins(
	ctx context.Context,
	req *connect.Request[organizationv1.ListPluginsRequest],
) (*connect.Response[organizationv1.ListPluginsResponse], error) {
	var (
		plugins    []db.PluginListRow
		tags       []db.PluginTagsListRow
		categories []db.PluginCategoriesListRow
	)

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		plugins, err = s.queries.PluginList(ctx)
		if err != nil {
			return fmt.Errorf("querying plugins: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		tags, err = s.queries.PluginTagsList(ctx)
		if err != nil {
			return fmt.Errorf("querying plugin tags: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		categories, err = s.queries.PluginCategoriesList(ctx)
		if err != nil {
			return fmt.Errorf("querying plugin categories: %w", err)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list plugins: %w", err))
	}

	return connect.NewResponse(&organizationv1.ListPluginsResponse{
		Plugins: adapter.FromPlugins(plugins, tags, categories),
	}), nil
}
