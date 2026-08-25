package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/marketplace-api/pkg/db/gen"
	catalogv1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/catalog/v1"
	marketplacev1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/marketplace/v1"
)

func (s *Server) ListPlugins(
	ctx context.Context,
	req *catalogv1.ListPluginsRequest,
) (*catalogv1.ListPluginsResponse, error) {
	params := db.PluginListParams{
		Sort: sortKey(req.GetSort()),
	}
	if query := req.GetQuery(); query != "" {
		params.Query = pgtype.Text{String: escapeLikePattern(query), Valid: true}
	}
	if categoryID := req.GetCategoryId(); categoryID != "" {
		params.CategoryID = pgtype.UUID{Bytes: uuid.MustParse(categoryID), Valid: true}
	}

	rows, err := s.queries.PluginList(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing plugins: %w", err))
	}

	// One query each for tags, categories and labels rather than three per row.
	tags, err := s.queries.PluginTagsList(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing plugin tags: %w", err))
	}
	categories, err := s.queries.PluginCategoriesList(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing plugin categories: %w", err))
	}
	labels, err := s.queries.PluginLabelsList(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing plugin labels: %w", err))
	}

	tagsByPlugin := map[uuid.UUID][]string{}
	for _, tag := range tags {
		tagsByPlugin[tag.PluginID] = append(tagsByPlugin[tag.PluginID], tag.Name)
	}
	categoriesByPlugin := map[uuid.UUID][]string{}
	for _, category := range categories {
		categoriesByPlugin[category.PluginID] = append(categoriesByPlugin[category.PluginID], category.ID.String())
	}
	labelsByPlugin := map[uuid.UUID][]catalogv1.PluginLabel{}
	for _, label := range labels {
		labelsByPlugin[label.PluginID] = append(labelsByPlugin[label.PluginID], labelFromDB(label.Name))
	}

	plugins := make([]*catalogv1.PluginSummary, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		plugins = append(plugins, catalogv1.PluginSummary_builder{
			Id:               row.ID.String(),
			Name:             row.Name,
			DisplayName:      row.DisplayName,
			DescriptionShort: row.DescriptionShort,
			OrganizationId:   row.OrganizationID.String(),
			Image:            row.Image,
			CategoryIds:      categoriesByPlugin[row.ID],
			Tags:             tagsByPlugin[row.ID],
			Labels:           labelsByPlugin[row.ID],
			LatestVersionId:  uuidOrEmpty(row.LatestVersionID),
			Published:        timestampOrNil(row.Published),
		}.Build())
	}

	return catalogv1.ListPluginsResponse_builder{
		Plugins: plugins,
	}.Build(), nil
}

// Search terms are substrings, not patterns: without this a query of "50%"
// matches every listing containing "50", and "a_b" matches "axb". Backslash is
// Postgres' default LIKE escape character, so no ESCAPE clause is needed.
var likePatternEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func escapeLikePattern(query string) string {
	return likePatternEscaper.Replace(query)
}

// RLS guarantees every visible plugin has a published definition, so this is
// never uuid.Nil in practice — guard anyway rather than emit an all-zeros UUID.
func uuidOrEmpty(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}

func (s *Server) GetPlugin(
	ctx context.Context,
	req *catalogv1.GetPluginRequest,
) (*catalogv1.GetPluginResponse, error) {
	pluginID := uuid.MustParse(req.GetPluginId())

	row, err := s.queries.PluginGetByID(ctx, db.PluginGetByIDParams{ID: pluginID})
	if errors.Is(err, pgx.ErrNoRows) {
		// RLS hides unpublished, restricted and soft-deleted listings, so "no
		// rows" and "not public" are indistinguishable here — deliberately.
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("plugin not found"))
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("getting plugin: %w", err))
	}

	tags, err := s.queries.PluginTagsListByPluginID(ctx, db.PluginTagsListByPluginIDParams{PluginID: pluginID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing plugin tags: %w", err))
	}
	categories, err := s.queries.PluginCategoriesListByPluginID(ctx, db.PluginCategoriesListByPluginIDParams{PluginID: pluginID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing plugin categories: %w", err))
	}
	labels, err := s.queries.PluginLabelsListByPluginID(ctx, db.PluginLabelsListByPluginIDParams{PluginID: pluginID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing plugin labels: %w", err))
	}
	features, err := s.queries.PluginFeaturesListByPluginID(ctx, db.PluginFeaturesListByPluginIDParams{PluginID: pluginID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing plugin features: %w", err))
	}
	links, err := s.queries.PluginDocumentationLinksList(ctx, db.PluginDocumentationLinksListParams{PluginID: pluginID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing documentation links: %w", err))
	}

	var capabilities []string
	var permissions []*marketplacev1.PluginPermission
	manifest, err := s.queries.PluginLatestPublishedDefinition(ctx, db.PluginLatestPublishedDefinitionParams{PluginID: pluginID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Unreachable while RLS requires a published version, but harmless.
	case err != nil:
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("loading plugin manifest: %w", err))
	default:
		capabilities, permissions, err = capabilitiesAndPermissions(manifest)
		if err != nil {
			// Manifests outlive the binary that parses them, and the rest of
			// the page is column-backed, so degrade rather than fail.
			s.logger.WarnContext(ctx, "unparseable plugin manifest",
				"plugin_id", pluginID, "error", err)
			capabilities, permissions = nil, nil
		}
	}

	tagNames := make([]string, 0, len(tags))
	for _, tag := range tags {
		tagNames = append(tagNames, tag.Name)
	}
	categoryIDs := make([]string, 0, len(categories))
	for _, category := range categories {
		categoryIDs = append(categoryIDs, category.ID.String())
	}
	pluginLabels := make([]catalogv1.PluginLabel, 0, len(labels))
	for _, label := range labels {
		pluginLabels = append(pluginLabels, labelFromDB(label.Name))
	}
	featureBlocks := make([]*marketplacev1.FeatureBlock, 0, len(features))
	for _, feature := range features {
		featureBlocks = append(featureBlocks, marketplacev1.FeatureBlock_builder{
			Title: feature.Title,
			Body:  feature.Body,
		}.Build())
	}
	documentationLinks := make([]*marketplacev1.DocumentationLink, 0, len(links))
	for _, link := range links {
		documentationLinks = append(documentationLinks, marketplacev1.DocumentationLink_builder{
			Id:      link.ID.String(),
			Title:   link.Title,
			UrlName: link.UrlName,
			Url:     link.Url,
		}.Build())
	}

	return catalogv1.GetPluginResponse_builder{
		Plugin: catalogv1.PluginDetails_builder{
			Id:                 row.ID.String(),
			Name:               row.Name,
			DisplayName:        row.DisplayName,
			DescriptionShort:   row.DescriptionShort,
			OrganizationId:     row.OrganizationID.String(),
			Image:              row.Image,
			CategoryIds:        categoryIDs,
			Tags:               tagNames,
			Labels:             pluginLabels,
			LatestVersionId:    uuidOrEmpty(row.LatestVersionID),
			Published:          timestampOrNil(row.Published),
			Description:        row.Description,
			AuthorName:         textOrEmpty(row.AuthorName),
			AuthorUrl:          textOrEmpty(row.AuthorUrl),
			RepositoryUrl:      textOrEmpty(row.RepositoryUrl),
			License:            row.License,
			Capabilities:       capabilities,
			Permissions:        permissions,
			Features:           featureBlocks,
			DocumentationLinks: documentationLinks,
		}.Build(),
	}.Build(), nil
}

// RLS restricts appstore.plugin_definitions to published, non-deleted rows for
// this role, so no filtering is repeated here.
func (s *Server) ListPluginVersions(
	ctx context.Context,
	req *catalogv1.ListPluginVersionsRequest,
) (*catalogv1.ListPluginVersionsResponse, error) {
	rows, err := s.queries.PluginVersionListByPluginID(ctx, db.PluginVersionListByPluginIDParams{
		PluginID: uuid.MustParse(req.GetPluginId()),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing plugin versions: %w", err))
	}

	versions := make([]*catalogv1.PublishedVersion, 0, len(rows))
	for _, row := range rows {
		versions = append(versions, catalogv1.PublishedVersion_builder{
			Id:             row.ID.String(),
			Version:        row.PluginVersion,
			Published:      timestamptzOrNil(row.Published),
			DefinitionHash: row.Hash,
			ReleaseNotes:   row.ReleaseNotes,
		}.Build())
	}

	return catalogv1.ListPluginVersionsResponse_builder{
		Versions: versions,
	}.Build(), nil
}

func textOrEmpty(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
