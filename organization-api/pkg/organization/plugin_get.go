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

func (s *Server) GetPluginDetail(
	ctx context.Context,
	req *connect.Request[organizationv1.GetPluginDetailRequest],
) (*connect.Response[organizationv1.GetPluginDetailResponse], error) {
	pluginID, err := uuid.Parse(req.Msg.GetPluginId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid plugin_id: %w", err))
	}

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
		docLinks   []db.AppstorePluginDocumentationLink
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

	return connect.NewResponse(organizationv1.GetPluginDetailResponse_builder{
		Plugin: pluginDetailFromRow(&plugin, tags, categories, docLinks),
	}.Build()), nil
}

func pluginDetailFromRow(
	plugin *db.PluginGetByIDRow,
	tags []db.PluginTagsListByPluginIDRow,
	categories []db.PluginCategoriesListByPluginIDRow,
	docLinks []db.AppstorePluginDocumentationLink,
) *organizationv1.PluginDetail {
	protoTags := make([]*organizationv1.Tag, 0, len(tags))
	for _, t := range tags {
		protoTags = append(protoTags, organizationv1.Tag_builder{
			Id:   t.ID.String(),
			Name: t.Name,
		}.Build())
	}

	protoCategories := make([]*organizationv1.Category, 0, len(categories))
	for _, c := range categories {
		protoCategories = append(protoCategories, organizationv1.Category_builder{
			Id:   c.ID.String(),
			Name: c.Name,
		}.Build())
	}

	protoDocLinks := make([]*organizationv1.DocumentationLink, 0, len(docLinks))
	for _, d := range docLinks {
		protoDocLinks = append(protoDocLinks, organizationv1.DocumentationLink_builder{
			Id:      d.ID.String(),
			Title:   d.Title,
			UrlName: d.UrlName,
			Url:     d.Url,
		}.Build())
	}

	var author *organizationv1.Author
	if plugin.AuthorName.Valid || plugin.AuthorUrl.Valid {
		authorBuilder := organizationv1.Author_builder{}
		if plugin.AuthorName.Valid {
			authorBuilder.Name = plugin.AuthorName.String
		}
		if plugin.AuthorUrl.Valid {
			authorBuilder.Url = plugin.AuthorUrl.String
		}
		author = authorBuilder.Build()
	}

	repositoryUrl := ""
	if plugin.RepositoryUrl.Valid {
		repositoryUrl = plugin.RepositoryUrl.String
	}

	return organizationv1.PluginDetail_builder{
		Id:                 plugin.ID.String(),
		Name:               plugin.Name,
		Description:        plugin.Description,
		DescriptionShort:   plugin.DescriptionShort,
		Tags:               protoTags,
		Categories:         protoCategories,
		Author:             author,
		RepositoryUrl:      repositoryUrl,
		DocumentationLinks: protoDocLinks,
	}.Build()
}
