package adapter

import (
	"github.com/google/uuid"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func FromPlugins(
	plugins []db.PluginListRow,
	tags []db.PluginTagsListRow,
	categories []db.PluginCategoriesListRow,
) []*organizationv1.PluginSummary {
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
			Id:          plugins[i].ID.String(),
			Name:        plugins[i].Name,
			Description: plugins[i].Description,
			Tags:        tagsByPlugin[plugins[i].ID],
			Categories:  categoriesByPlugin[plugins[i].ID],
		})
	}
	return result
}

func FromPluginDetail(
	plugin db.PluginGetByIDRow,
	tags []db.PluginTagsListByPluginIDRow,
	categories []db.PluginCategoriesListByPluginIDRow,
	docLinks []db.ZappstorePluginDocumentationLink,
) *organizationv1.PluginDetail {
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

	return &organizationv1.PluginDetail{
		Id:                 plugin.ID.String(),
		Name:               plugin.Name,
		Description:        plugin.Description,
		Tags:               protoTags,
		Categories:         protoCategories,
		Author:             author,
		RepositoryUrl:      repositoryUrl,
		DocumentationLinks: protoDocLinks,
	}
}
