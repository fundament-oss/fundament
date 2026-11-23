package pluginlisting

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/google/uuid"

	db "github.com/fundament-oss/fundament/common/pluginlisting/db/gen"
)

// Draft holds the listing fields a publisher edits through UpdatePlugin. They
// wait in appstore.plugins.listing_draft until a version is submitted, and go
// live when that version is approved. Visibility and allowed organizations are
// not part of it: they take effect immediately.
type Draft struct {
	Description        string      `json:"description"`
	CategoryIDs        []uuid.UUID `json:"categoryIds"`
	Image              string      `json:"image"`
	AuthorName         string      `json:"authorName"`
	AuthorURL          string      `json:"authorUrl"`
	Logo               string      `json:"logo"`     // data: URI; empty shows the version's
	LogoDark           string      `json:"logoDark"` // data: URI; empty shows the version's
	Features           []Feature   `json:"features"`
	DocumentationLinks []Link      `json:"documentationLinks"`
}

// Feature is one feature block. A nil ID is a block that does not exist yet.
type Feature struct {
	ID    uuid.UUID `json:"id"`
	Title string    `json:"title"`
	Body  string    `json:"body"`
}

// Link is one documentation link. A nil ID is a link that does not exist yet.
type Link struct {
	ID      uuid.UUID `json:"id"`
	Title   string    `json:"title"`
	URLName string    `json:"urlName"`
	URL     string    `json:"url"`
}

// Equal reports whether two drafts would leave the listing the same.
func (d *Draft) Equal(other *Draft) bool {
	if d == nil || other == nil {
		return d == other
	}
	return d.Description == other.Description &&
		slices.Equal(d.CategoryIDs, other.CategoryIDs) &&
		d.Image == other.Image &&
		d.AuthorName == other.AuthorName &&
		d.AuthorURL == other.AuthorURL &&
		d.Logo == other.Logo &&
		d.LogoDark == other.LogoDark &&
		slices.Equal(d.Features, other.Features) &&
		slices.Equal(d.DocumentationLinks, other.DocumentationLinks)
}

// ParseDraft decodes a stored draft; nil or empty input means none.
func ParseDraft(raw []byte) (*Draft, error) {
	if len(raw) == 0 {
		return nil, nil //nolint:nilnil // no draft is not an error
	}
	var draft Draft
	if err := json.Unmarshal(raw, &draft); err != nil {
		return nil, fmt.Errorf("decoding listing draft: %w", err)
	}
	return &draft, nil
}

// apply makes draft the live listing. Rows keep their IDs; a row the draft
// still names but that is gone by now is added again rather than failing the
// approval.
func apply(ctx context.Context, q *db.Queries, pluginID uuid.UUID, draft *Draft) error {
	if err := q.PluginApplyDraftContent(ctx, db.PluginApplyDraftContentParams{
		ID:          pluginID,
		Description: draft.Description,
		Image:       draft.Image,
		AuthorName:  optionalText(draft.AuthorName),
		AuthorUrl:   optionalText(draft.AuthorURL),
		Logo:        draft.Logo,
		LogoDark:    draft.LogoDark,
	}); err != nil {
		return fmt.Errorf("updating listing: %w", err)
	}

	if err := q.PluginCategoriesDelete(ctx, db.PluginCategoriesDeleteParams{PluginID: pluginID}); err != nil {
		return fmt.Errorf("clearing categories: %w", err)
	}
	for _, categoryID := range draft.CategoryIDs {
		if err := q.PluginCategoryInsert(ctx, db.PluginCategoryInsertParams{PluginID: pluginID, CategoryID: categoryID}); err != nil {
			return fmt.Errorf("adding category: %w", err)
		}
	}

	if err := applyFeatures(ctx, q, pluginID, draft.Features); err != nil {
		return err
	}
	return applyLinks(ctx, q, pluginID, draft.DocumentationLinks)
}

func applyFeatures(ctx context.Context, q *db.Queries, pluginID uuid.UUID, features []Feature) error {
	kept := make([]uuid.UUID, 0, len(features))
	for _, feature := range features {
		if feature.ID != uuid.Nil {
			kept = append(kept, feature.ID)
		}
	}
	if err := q.PluginFeaturesSoftDeleteNotIn(ctx, db.PluginFeaturesSoftDeleteNotInParams{PluginID: pluginID, KeptIds: kept}); err != nil {
		return fmt.Errorf("clearing features: %w", err)
	}
	for i, feature := range features {
		position := int32(i) //nolint:gosec // at most 20
		if feature.ID != uuid.Nil {
			updated, err := q.PluginFeatureUpdate(ctx, db.PluginFeatureUpdateParams{
				ID: feature.ID, PluginID: pluginID, Title: feature.Title, Body: feature.Body, Position: position,
			})
			if err != nil {
				return fmt.Errorf("updating feature: %w", err)
			}
			if updated > 0 {
				continue
			}
		}
		if err := q.PluginFeatureInsert(ctx, db.PluginFeatureInsertParams{
			PluginID: pluginID, Title: feature.Title, Body: feature.Body, Position: position,
		}); err != nil {
			return fmt.Errorf("adding feature: %w", err)
		}
	}
	return nil
}

func applyLinks(ctx context.Context, q *db.Queries, pluginID uuid.UUID, links []Link) error {
	kept := make([]uuid.UUID, 0, len(links))
	for _, link := range links {
		if link.ID != uuid.Nil {
			kept = append(kept, link.ID)
		}
	}
	if err := q.PluginDocLinksSoftDeleteNotIn(ctx, db.PluginDocLinksSoftDeleteNotInParams{PluginID: pluginID, KeptIds: kept}); err != nil {
		return fmt.Errorf("clearing documentation links: %w", err)
	}
	for i, link := range links {
		position := int32(i) //nolint:gosec // at most 20
		if link.ID != uuid.Nil {
			updated, err := q.PluginDocLinkUpdate(ctx, db.PluginDocLinkUpdateParams{
				ID: link.ID, PluginID: pluginID, Title: link.Title, UrlName: link.URLName, Url: link.URL, Position: position,
			})
			if err != nil {
				return fmt.Errorf("updating documentation link: %w", err)
			}
			if updated > 0 {
				continue
			}
		}
		if err := q.PluginDocLinkInsert(ctx, db.PluginDocLinkInsertParams{
			PluginID: pluginID, Title: link.Title, UrlName: link.URLName, Url: link.URL, Position: position,
		}); err != nil {
			return fmt.Errorf("adding documentation link: %w", err)
		}
	}
	return nil
}
