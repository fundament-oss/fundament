package pluginlisting

import (
	"github.com/google/uuid"

	marketplacev1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/marketplace/v1"
)

// Proto renders the draft for the API; nil for no draft.
func (d *Draft) Proto() *marketplacev1.ListingDraft {
	if d == nil {
		return nil
	}

	categoryIDs := make([]string, 0, len(d.CategoryIDs))
	for _, id := range d.CategoryIDs {
		categoryIDs = append(categoryIDs, id.String())
	}
	features := make([]*marketplacev1.FeatureBlock, 0, len(d.Features))
	for _, feature := range d.Features {
		features = append(features, marketplacev1.FeatureBlock_builder{
			Id: idOrEmpty(feature.ID), Title: feature.Title, Body: feature.Body,
		}.Build())
	}
	links := make([]*marketplacev1.DocumentationLink, 0, len(d.DocumentationLinks))
	for _, link := range d.DocumentationLinks {
		links = append(links, marketplacev1.DocumentationLink_builder{
			Id: idOrEmpty(link.ID), Title: link.Title, UrlName: link.URLName, Url: link.URL,
		}.Build())
	}

	return marketplacev1.ListingDraft_builder{
		Description:        d.Description,
		CategoryIds:        categoryIDs,
		Image:              d.Image,
		AuthorName:         d.AuthorName,
		AuthorUrl:          d.AuthorURL,
		Logo:               d.Logo,
		LogoDark:           d.LogoDark,
		Features:           features,
		DocumentationLinks: links,
	}.Build()
}

func idOrEmpty(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}
