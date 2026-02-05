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

	// Build maps of tags and categories by plugin ID
	tagsByPlugin := make(map[uuid.UUID][]*organizationv1.Tag)
	for _, t := range tags {
		tagsByPlugin[t.PluginID] = append(tagsByPlugin[t.PluginID], &organizationv1.Tag{
			Id:   t.ID.String(),
			Name: t.Name,
		})
	}

	categoriesByPlugin := make(map[uuid.UUID][]*organizationv1.Category)
	for _, c := range categories {
		categoriesByPlugin[c.PluginID] = append(categoriesByPlugin[c.PluginID], &organizationv1.Category{
			Id:   c.ID.String(),
			Name: c.Name,
		})
	}

	result := make([]*organizationv1.PluginSummary, 0, len(plugins))
	for i := range plugins {
		result = append(result, &organizationv1.PluginSummary{
			Id:               plugins[i].ID.String(),
			Name:             plugins[i].Name,
			Description:      plugins[i].Description,
			DescriptionShort: plugins[i].DescriptionShort,
			Tags:             tagsByPlugin[plugins[i].ID],
			Categories:       categoriesByPlugin[plugins[i].ID],
		})
	}

	return connect.NewResponse(&organizationv1.ListPluginsResponse{
		Plugins: result,
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

	// Build tags list
	protoTags := make([]*organizationv1.Tag, 0, len(tags))
	for _, t := range tags {
		protoTags = append(protoTags, &organizationv1.Tag{
			Id:   t.ID.String(),
			Name: t.Name,
		})
	}

	// Build categories list
	protoCategories := make([]*organizationv1.Category, 0, len(categories))
	for _, c := range categories {
		protoCategories = append(protoCategories, &organizationv1.Category{
			Id:   c.ID.String(),
			Name: c.Name,
		})
	}

	// Build documentation links list
	protoDocLinks := make([]*organizationv1.DocumentationLink, 0, len(docLinks))
	for _, d := range docLinks {
		protoDocLinks = append(protoDocLinks, &organizationv1.DocumentationLink{
			Id:      d.ID.String(),
			Title:   d.Title,
			UrlName: d.UrlName,
			Url:     d.Url,
		})
	}

	// Build author if present
	var author *organizationv1.Author
	if plugin.AuthorName.Valid || plugin.AuthorUrl.Valid {
		author = &organizationv1.Author{}
		if plugin.AuthorName.Valid {
			author.Name = plugin.AuthorName.String
		}
		if plugin.AuthorUrl.Valid {
			author.Url = plugin.AuthorUrl.String
		}
	}

	// Build repository URL
	repositoryUrl := ""
	if plugin.RepositoryUrl.Valid {
		repositoryUrl = plugin.RepositoryUrl.String
	}

	return connect.NewResponse(&organizationv1.GetPluginDetailResponse{
		Plugin: &organizationv1.PluginDetail{
			Id:                 plugin.ID.String(),
			Name:               plugin.Name,
			Description:        plugin.Description,
			DescriptionShort:   plugin.DescriptionShort,
			Tags:               protoTags,
			Categories:         protoCategories,
			Author:             author,
			RepositoryUrl:      repositoryUrl,
			DocumentationLinks: protoDocLinks,
		},
	}), nil
}
