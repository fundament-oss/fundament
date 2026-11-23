package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/common/pluginlisting"
	marketplacev1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/marketplace/v1"
	registryv1 "github.com/fundament-oss/fundament/marketplace-registry-api/pkg/proto/gen/registry/v1"
)

// storedDraft is the listing_draft UpdatePlugin stores: the edited fields, or
// nil when they leave the live listing as it is.
func (s *Server) storedDraft(ctx context.Context, req *registryv1.UpdatePluginRequest, live *registryv1.Plugin) ([]byte, error) {
	draft, err := s.draftFromRequest(ctx, req, live)
	if err != nil {
		return nil, err
	}
	current, err := liveDraft(live)
	if err != nil {
		return nil, err
	}
	if draft.Equal(current) {
		return nil, nil
	}
	raw, err := json.Marshal(draft)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("encoding listing draft: %w", err))
	}
	return raw, nil
}

// draftFromRequest checks the request against the live listing, since the draft
// is applied long after this call: ids must name live rows and categories must
// exist.
func (s *Server) draftFromRequest(ctx context.Context, req *registryv1.UpdatePluginRequest, live *registryv1.Plugin) (*pluginlisting.Draft, error) {
	logo, err := uploadedLogo("logo", req.GetLogo())
	if err != nil {
		return nil, err
	}
	logoDark, err := uploadedLogo("logo_dark", req.GetLogoDark())
	if err != nil {
		return nil, err
	}

	categoryIDs, err := s.knownCategories(ctx, req.GetCategoryIds())
	if err != nil {
		return nil, err
	}

	features, err := draftFeatures(req.GetFeatures(), live.GetFeatures())
	if err != nil {
		return nil, err
	}
	links, err := draftLinks(req.GetDocumentationLinks(), live.GetDocumentationLinks())
	if err != nil {
		return nil, err
	}

	return &pluginlisting.Draft{
		Description:        req.GetDescription(),
		CategoryIDs:        categoryIDs,
		Image:              req.GetImage(),
		AuthorName:         req.GetAuthorName(),
		AuthorURL:          req.GetAuthorUrl(),
		Logo:               logo,
		LogoDark:           logoDark,
		Features:           features,
		DocumentationLinks: links,
	}, nil
}

// knownCategories parses the requested categories, dropping repeats, and
// refuses one the vocabulary does not hold.
func (s *Server) knownCategories(ctx context.Context, requested []string) ([]uuid.UUID, error) {
	rows, err := s.queries.CategoryList(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing categories: %w", err))
	}
	known := make(map[uuid.UUID]bool, len(rows))
	for _, row := range rows {
		known[row.ID] = true
	}

	ids := make([]uuid.UUID, 0, len(requested))
	for _, value := range requested {
		id := uuid.MustParse(value)
		if !known[id] {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unknown category %s", value))
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func draftFeatures(requested, live []*marketplacev1.FeatureBlock) ([]pluginlisting.Feature, error) {
	ids, err := draftRowIDs(requested, live, "feature block")
	if err != nil {
		return nil, err
	}
	features := make([]pluginlisting.Feature, 0, len(requested))
	for i, feature := range requested {
		features = append(features, pluginlisting.Feature{ID: ids[i], Title: feature.GetTitle(), Body: feature.GetBody()})
	}
	return features, nil
}

func draftLinks(requested, live []*marketplacev1.DocumentationLink) ([]pluginlisting.Link, error) {
	ids, err := draftRowIDs(requested, live, "documentation link")
	if err != nil {
		return nil, err
	}
	links := make([]pluginlisting.Link, 0, len(requested))
	for i, link := range requested {
		links = append(links, pluginlisting.Link{ID: ids[i], Title: link.GetTitle(), URLName: link.GetUrlName(), URL: link.GetUrl()})
	}
	return links, nil
}

// draftRowIDs is the id of each requested row, uuid.Nil for a new one. An id
// must name a live row of this listing, and at most once: two entries claiming
// one row would collapse into it.
func draftRowIDs[T interface{ GetId() string }](requested, live []T, kind string) ([]uuid.UUID, error) {
	liveIDs := make(map[string]bool, len(live))
	for _, row := range live {
		liveIDs[row.GetId()] = true
	}

	ids := make([]uuid.UUID, 0, len(requested))
	for _, row := range requested {
		if row.GetId() == "" {
			ids = append(ids, uuid.Nil)
			continue
		}
		if !liveIDs[row.GetId()] {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unknown %s %s", kind, row.GetId()))
		}
		id := uuid.MustParse(row.GetId())
		if slices.Contains(ids, id) {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%s %s appears twice", kind, row.GetId()))
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// liveDraft is the live listing in draft form, to tell an edit from a no-op.
func liveDraft(live *registryv1.Plugin) (*pluginlisting.Draft, error) {
	categoryIDs := make([]uuid.UUID, 0, len(live.GetCategoryIds()))
	for _, id := range live.GetCategoryIds() {
		categoryIDs = append(categoryIDs, uuid.MustParse(id))
	}
	features, err := draftFeatures(live.GetFeatures(), live.GetFeatures())
	if err != nil {
		return nil, err
	}
	links, err := draftLinks(live.GetDocumentationLinks(), live.GetDocumentationLinks())
	if err != nil {
		return nil, err
	}
	return &pluginlisting.Draft{
		Description:        live.GetDescription(),
		CategoryIDs:        categoryIDs,
		Image:              live.GetImage(),
		AuthorName:         live.GetAuthorName(),
		AuthorURL:          live.GetAuthorUrl(),
		Logo:               live.GetListingLogo(),
		LogoDark:           live.GetListingLogoDark(),
		Features:           features,
		DocumentationLinks: links,
	}, nil
}

// pendingListing is the stored draft as Plugin.pending_listing; nil when the
// listing has none.
func pendingListing(raw []byte) (*marketplacev1.ListingDraft, error) {
	draft, err := pluginlisting.ParseDraft(raw)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return draft.Proto(), nil
}
