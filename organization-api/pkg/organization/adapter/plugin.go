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
) []*organizationv1.Plugin {
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

	result := make([]*organizationv1.Plugin, 0, len(plugins))
	for i := range plugins {
		result = append(result, &organizationv1.Plugin{
			Id:          plugins[i].ID.String(),
			Name:        plugins[i].Name,
			Description: plugins[i].Description,
			Tags:        tagsByPlugin[plugins[i].ID],
			Categories:  categoriesByPlugin[plugins[i].ID],
		})
	}
	return result
}
