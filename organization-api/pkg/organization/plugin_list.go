package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) ListPlugins(
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

	tagsByPlugin := buildTagsByPlugin(tags)
	categoriesByPlugin := buildCategoriesByPlugin(categories)

	result := make([]*organizationv1.PluginSummary, 0, len(plugins))
	for i := range plugins {
		result = append(result, pluginSummaryFromRow(&plugins[i], tagsByPlugin, categoriesByPlugin))
	}

	return connect.NewResponse(&organizationv1.ListPluginsResponse{
		Plugins: result,
	}), nil
}

func buildTagsByPlugin(tags []db.PluginTagsListRow) map[uuid.UUID][]*organizationv1.Tag {
	tagsByPlugin := make(map[uuid.UUID][]*organizationv1.Tag)
	for _, t := range tags {
		tagsByPlugin[t.PluginID] = append(tagsByPlugin[t.PluginID], &organizationv1.Tag{
			Id:   t.ID.String(),
			Name: t.Name,
		})
	}
	return tagsByPlugin
}

func buildCategoriesByPlugin(categories []db.PluginCategoriesListRow) map[uuid.UUID][]*organizationv1.Category {
	categoriesByPlugin := make(map[uuid.UUID][]*organizationv1.Category)
	for _, c := range categories {
		categoriesByPlugin[c.PluginID] = append(categoriesByPlugin[c.PluginID], &organizationv1.Category{
			Id:   c.ID.String(),
			Name: c.Name,
		})
	}
	return categoriesByPlugin
}

func pluginSummaryFromRow(
	row *db.PluginListRow,
	tagsByPlugin map[uuid.UUID][]*organizationv1.Tag,
	categoriesByPlugin map[uuid.UUID][]*organizationv1.Category,
) *organizationv1.PluginSummary {
	return &organizationv1.PluginSummary{
		Id:               row.ID.String(),
		Name:             row.Name,
		Description:      row.Description,
		DescriptionShort: row.DescriptionShort,
		Tags:             tagsByPlugin[row.ID],
		Categories:       categoriesByPlugin[row.ID],
	}
}
