package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

func (s *OrganizationServer) GetPluginDetail(
	ctx context.Context,
	req *connect.Request[organizationv1.GetPluginDetailRequest],
) (*connect.Response[organizationv1.GetPluginDetailResponse], error) {
	pluginID, err := uuid.Parse(req.Msg.PluginId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid plugin_id: %w", err))
	}

	// Fetch the plugin first to check if it exists
	plugin, err := s.queries.PluginGetByID(ctx, db.PluginGetByIDParams{ID: pluginID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("plugin not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get plugin: %w", err))
	}

	var (
		tags       []db.PluginTagsListByPluginIDRow
		categories []db.PluginCategoriesListByPluginIDRow
		docLinks   []db.ZappstorePluginDocumentationLink
	)

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		tags, err = s.queries.PluginTagsListByPluginID(ctx, db.PluginTagsListByPluginIDParams{PluginID: pluginID})
		if err != nil {
			return fmt.Errorf("querying plugin tags: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		categories, err = s.queries.PluginCategoriesListByPluginID(ctx, db.PluginCategoriesListByPluginIDParams{PluginID: pluginID})
		if err != nil {
			return fmt.Errorf("querying plugin categories: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		docLinks, err = s.queries.PluginDocumentationLinksList(ctx, db.PluginDocumentationLinksListParams{PluginID: pluginID})
		if err != nil {
			return fmt.Errorf("querying plugin documentation links: %w", err)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get plugin details: %w", err))
	}

	return connect.NewResponse(&organizationv1.GetPluginDetailResponse{
		Plugin: adapter.FromPluginDetail(plugin, tags, categories, docLinks),
	}), nil
}
