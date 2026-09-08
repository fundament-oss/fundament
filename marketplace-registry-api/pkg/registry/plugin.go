package registry

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/rollback"
	marketplacev1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/marketplace/v1"
	db "github.com/fundament-oss/fundament/marketplace-registry-api/pkg/db/gen"
	registryv1 "github.com/fundament-oss/fundament/marketplace-registry-api/pkg/proto/gen/registry/v1"
)

func (s *Server) ListPlugins(
	ctx context.Context,
	_ *registryv1.ListPluginsRequest,
) (*registryv1.ListPluginsResponse, error) {
	rows, err := s.queries.PluginList(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing plugins: %w", err))
	}

	pluginIDs := make([]uuid.UUID, 0, len(rows))
	for i := range rows {
		pluginIDs = append(pluginIDs, rows[i].ID)
	}

	children, err := s.loadPluginChildren(ctx, pluginIDs)
	if err != nil {
		return nil, err
	}

	plugins := make([]*registryv1.Plugin, 0, len(rows))
	for i := range rows {
		plugins = append(plugins, pluginFromRow(pluginRowFromList(&rows[i]), children))
	}

	return registryv1.ListPluginsResponse_builder{Plugins: plugins}.Build(), nil
}

func (s *Server) GetPlugin(
	ctx context.Context,
	req *registryv1.GetPluginRequest,
) (*registryv1.GetPluginResponse, error) {
	row, err := s.queries.PluginGetByID(ctx, db.PluginGetByIDParams{
		ID: uuid.MustParse(req.GetPluginId()),
	})
	if err != nil {
		return nil, pluginLookupError(err)
	}

	children, err := s.loadPluginChildren(ctx, []uuid.UUID{row.ID})
	if err != nil {
		return nil, err
	}

	return registryv1.GetPluginResponse_builder{
		Plugin: pluginFromRow(pluginRowFromGet(&row), children),
	}.Build(), nil
}

func (s *Server) CreatePlugin(
	ctx context.Context,
	req *registryv1.CreatePluginRequest,
) (*registryv1.CreatePluginResponse, error) {
	organizationID, ok := OrganizationIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, errors.New("no organization in context"))
	}

	if err := checkPluginName(req.GetName()); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("begin transaction: %w", err))
	}
	defer rollback.Rollback(ctx, tx, s.logger)
	qtx := s.queries.WithTx(tx)

	pluginID, err := qtx.PluginCreate(ctx, db.PluginCreateParams{
		OrganizationID:   organizationID,
		Name:             req.GetName(),
		DisplayName:      req.GetDisplayName(),
		DescriptionShort: req.GetDescriptionShort(),
		Description:      req.GetDescription(),
		Image:            req.GetImage(),
		RepositoryUrl:    textOrNull(req.GetRepositoryUrl()),
		License:          req.GetLicense(),
		Visibility:       visibilityToDB(req.GetVisibility()),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return nil, connect.NewError(connect.CodeAlreadyExists,
				fmt.Errorf("plugin %q already exists in this organization", req.GetName()))
		}
		return nil, writeError(err, "creating plugin")
	}

	if err := s.replaceCategories(ctx, qtx, pluginID, req.GetCategoryIds()); err != nil {
		return nil, err
	}
	if err := s.replaceTags(ctx, qtx, pluginID, req.GetTags()); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("commit transaction: %w", err))
	}

	plugin, err := s.loadPlugin(ctx, pluginID)
	if err != nil {
		return nil, err
	}

	return registryv1.CreatePluginResponse_builder{Plugin: plugin}.Build(), nil
}

func (s *Server) UpdatePlugin(
	ctx context.Context,
	req *registryv1.UpdatePluginRequest,
) (*registryv1.UpdatePluginResponse, error) {
	pluginID := uuid.MustParse(req.GetPluginId())

	// The update itself is RLS-scoped, but a listing the caller does not own
	// would silently affect zero rows and read back as NOT_FOUND only by
	// accident; looking it up first makes that explicit.
	if _, err := s.queries.PluginGetByID(ctx, db.PluginGetByIDParams{ID: pluginID}); err != nil {
		return nil, pluginLookupError(err)
	}

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("begin transaction: %w", err))
	}
	defer rollback.Rollback(ctx, tx, s.logger)
	qtx := s.queries.WithTx(tx)

	updated, err := qtx.PluginUpdate(ctx, db.PluginUpdateParams{
		ID:               pluginID,
		DisplayName:      req.GetDisplayName(),
		DescriptionShort: req.GetDescriptionShort(),
		Description:      req.GetDescription(),
		Image:            req.GetImage(),
		RepositoryUrl:    textOrNull(req.GetRepositoryUrl()),
		AuthorName:       textOrNull(req.GetAuthorName()),
		AuthorUrl:        textOrNull(req.GetAuthorUrl()),
		License:          req.GetLicense(),
		Visibility:       visibilityToDB(req.GetVisibility()),
	})
	if err != nil {
		return nil, writeError(err, "updating plugin")
	}
	// Otherwise the replacements below would commit against a listing deleted
	// since the lookup.
	if updated == 0 {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("plugin not found"))
	}

	// A full replacement: everything the request omits is cleared.
	if err := s.replaceCategories(ctx, qtx, pluginID, req.GetCategoryIds()); err != nil {
		return nil, err
	}
	if err := s.replaceTags(ctx, qtx, pluginID, req.GetTags()); err != nil {
		return nil, err
	}
	if err := s.replaceAllowedOrganizations(ctx, qtx, pluginID, req.GetAllowedOrganizationIds()); err != nil {
		return nil, err
	}
	if err := s.replaceDocumentationLinks(ctx, qtx, pluginID, req.GetDocumentationLinks()); err != nil {
		return nil, err
	}
	if err := s.replaceFeatures(ctx, qtx, pluginID, req.GetFeatures()); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("commit transaction: %w", err))
	}

	plugin, err := s.loadPlugin(ctx, pluginID)
	if err != nil {
		return nil, err
	}

	return registryv1.UpdatePluginResponse_builder{Plugin: plugin}.Build(), nil
}

// DeletePlugin soft-deletes the listing and everything hanging off it in one
// transaction. Leaving versions active would strand rows whose parent is gone,
// and leaving a submission open would leave a review queue entry pointing at a
// deleted listing.
func (s *Server) DeletePlugin(
	ctx context.Context,
	req *registryv1.DeletePluginRequest,
) (*registryv1.DeletePluginResponse, error) {
	pluginID := uuid.MustParse(req.GetPluginId())

	if _, err := s.queries.PluginGetByID(ctx, db.PluginGetByIDParams{ID: pluginID}); err != nil {
		return nil, pluginLookupError(err)
	}

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("begin transaction: %w", err))
	}
	defer rollback.Rollback(ctx, tx, s.logger)
	qtx := s.queries.WithTx(tx)

	if err := qtx.SubmissionCloseOpenByPluginID(ctx, db.SubmissionCloseOpenByPluginIDParams{
		PluginID: pluginID,
	}); err != nil {
		return nil, writeError(err, "closing open submissions")
	}
	if err := qtx.PluginVersionsSoftDeleteByPluginID(ctx, db.PluginVersionsSoftDeleteByPluginIDParams{
		PluginID: pluginID,
	}); err != nil {
		return nil, writeError(err, "deleting plugin versions")
	}
	if err := qtx.PluginSoftDelete(ctx, db.PluginSoftDeleteParams{ID: pluginID}); err != nil {
		return nil, writeError(err, "deleting plugin")
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("commit transaction: %w", err))
	}

	return registryv1.DeletePluginResponse_builder{}.Build(), nil
}

func (s *Server) ListCategories(
	ctx context.Context,
	_ *registryv1.ListCategoriesRequest,
) (*registryv1.ListCategoriesResponse, error) {
	rows, err := s.queries.CategoryList(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing categories: %w", err))
	}

	categories := make([]*marketplacev1.Category, 0, len(rows))
	for _, row := range rows {
		categories = append(categories, marketplacev1.Category_builder{
			Id:   row.ID.String(),
			Name: row.Name,
		}.Build())
	}

	return registryv1.ListCategoriesResponse_builder{Categories: categories}.Build(), nil
}

// registryPluginRow is the shape the list and get rows share, so the projection
// below is written once rather than per query.
type registryPluginRow struct {
	ID                       uuid.UUID
	Name                     string
	DisplayName              string
	DescriptionShort         string
	Description              string
	OrganizationID           uuid.UUID
	Image                    string
	AuthorName               string
	AuthorURL                string
	RepositoryURL            string
	License                  string
	Visibility               registryv1.PluginVisibility
	Created                  *timestamppb.Timestamp
	Updated                  *timestamppb.Timestamp
	LatestPublishedVersionID uuid.UUID
}

func pluginRowFromList(row *db.PluginListRow) *registryPluginRow {
	return &registryPluginRow{
		ID:                       row.ID,
		Name:                     row.Name,
		DisplayName:              row.DisplayName,
		DescriptionShort:         row.DescriptionShort,
		Description:              row.Description,
		OrganizationID:           row.OrganizationID,
		Image:                    row.Image,
		AuthorName:               textOrEmpty(row.AuthorName),
		AuthorURL:                textOrEmpty(row.AuthorUrl),
		RepositoryURL:            textOrEmpty(row.RepositoryUrl),
		License:                  row.License,
		Visibility:               visibilityFromDB(row.Visibility),
		Created:                  timestamptzOrNil(row.Created),
		Updated:                  timestamptzOrNil(row.Updated),
		LatestPublishedVersionID: row.LatestPublishedVersionID,
	}
}

func pluginRowFromGet(row *db.PluginGetByIDRow) *registryPluginRow {
	return &registryPluginRow{
		ID:                       row.ID,
		Name:                     row.Name,
		DisplayName:              row.DisplayName,
		DescriptionShort:         row.DescriptionShort,
		Description:              row.Description,
		OrganizationID:           row.OrganizationID,
		Image:                    row.Image,
		AuthorName:               textOrEmpty(row.AuthorName),
		AuthorURL:                textOrEmpty(row.AuthorUrl),
		RepositoryURL:            textOrEmpty(row.RepositoryUrl),
		License:                  row.License,
		Visibility:               visibilityFromDB(row.Visibility),
		Created:                  timestamptzOrNil(row.Created),
		Updated:                  timestamptzOrNil(row.Updated),
		LatestPublishedVersionID: row.LatestPublishedVersionID,
	}
}

// loadPlugin re-reads a listing after a write so the response reflects what was
// actually stored rather than what the request asked for.
func (s *Server) loadPlugin(ctx context.Context, pluginID uuid.UUID) (*registryv1.Plugin, error) {
	row, err := s.queries.PluginGetByID(ctx, db.PluginGetByIDParams{ID: pluginID})
	if err != nil {
		return nil, pluginLookupError(err)
	}

	children, err := s.loadPluginChildren(ctx, []uuid.UUID{row.ID})
	if err != nil {
		return nil, err
	}

	return pluginFromRow(pluginRowFromGet(&row), children), nil
}

// pluginChildren is the child rows of a set of listings, keyed by plugin id.
// One query per kind covers the whole set; per listing it was five queries a
// row, each a pool acquire that re-runs the RLS GUCs.
type pluginChildren struct {
	categories  map[uuid.UUID][]string
	tags        map[uuid.UUID][]string
	allowedOrgs map[uuid.UUID][]string
	links       map[uuid.UUID][]*marketplacev1.DocumentationLink
	features    map[uuid.UUID][]*marketplacev1.FeatureBlock
}

func (s *Server) loadPluginChildren(ctx context.Context, pluginIDs []uuid.UUID) (*pluginChildren, error) {
	children := &pluginChildren{
		categories:  map[uuid.UUID][]string{},
		tags:        map[uuid.UUID][]string{},
		allowedOrgs: map[uuid.UUID][]string{},
		links:       map[uuid.UUID][]*marketplacev1.DocumentationLink{},
		features:    map[uuid.UUID][]*marketplacev1.FeatureBlock{},
	}
	if len(pluginIDs) == 0 {
		return children, nil
	}

	categoryRows, err := s.queries.PluginCategoriesListByPluginIDs(ctx, db.PluginCategoriesListByPluginIDsParams{
		PluginIds: pluginIDs,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing plugin categories: %w", err))
	}
	for _, row := range categoryRows {
		children.categories[row.PluginID] = append(children.categories[row.PluginID], row.CategoryID.String())
	}

	tagRows, err := s.queries.PluginTagsListByPluginIDs(ctx, db.PluginTagsListByPluginIDsParams{
		PluginIds: pluginIDs,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing plugin tags: %w", err))
	}
	for _, row := range tagRows {
		children.tags[row.PluginID] = append(children.tags[row.PluginID], row.Name)
	}

	allowedOrgRows, err := s.queries.PluginAllowedOrgsListByPluginIDs(ctx, db.PluginAllowedOrgsListByPluginIDsParams{
		PluginIds: pluginIDs,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing allowed organizations: %w", err))
	}
	for _, row := range allowedOrgRows {
		children.allowedOrgs[row.PluginID] = append(children.allowedOrgs[row.PluginID], row.OrganizationID.String())
	}

	linkRows, err := s.queries.PluginDocLinksListByPluginIDs(ctx, db.PluginDocLinksListByPluginIDsParams{
		PluginIds: pluginIDs,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing documentation links: %w", err))
	}
	for _, row := range linkRows {
		children.links[row.PluginID] = append(children.links[row.PluginID], marketplacev1.DocumentationLink_builder{
			Id:      row.ID.String(),
			Title:   row.Title,
			UrlName: row.UrlName,
			Url:     row.Url,
		}.Build())
	}

	featureRows, err := s.queries.PluginFeaturesListByPluginIDs(ctx, db.PluginFeaturesListByPluginIDsParams{
		PluginIds: pluginIDs,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing feature blocks: %w", err))
	}
	for _, row := range featureRows {
		children.features[row.PluginID] = append(children.features[row.PluginID], marketplacev1.FeatureBlock_builder{
			Id:    row.ID.String(),
			Title: row.Title,
			Body:  row.Body,
		}.Build())
	}

	return children, nil
}

func pluginFromRow(row *registryPluginRow, children *pluginChildren) *registryv1.Plugin {
	return registryv1.Plugin_builder{
		Id:                       row.ID.String(),
		Name:                     row.Name,
		DisplayName:              row.DisplayName,
		DescriptionShort:         row.DescriptionShort,
		Description:              row.Description,
		OrganizationId:           row.OrganizationID.String(),
		Image:                    row.Image,
		CategoryIds:              children.categories[row.ID],
		Tags:                     children.tags[row.ID],
		AuthorName:               row.AuthorName,
		AuthorUrl:                row.AuthorURL,
		RepositoryUrl:            row.RepositoryURL,
		License:                  row.License,
		DocumentationLinks:       children.links[row.ID],
		Features:                 children.features[row.ID],
		Visibility:               row.Visibility,
		AllowedOrganizationIds:   children.allowedOrgs[row.ID],
		LatestPublishedVersionId: uuidOrEmpty(row.LatestPublishedVersionID),
		Created:                  row.Created,
		Updated:                  row.Updated,
	}.Build()
}

func (s *Server) replaceCategories(ctx context.Context, qtx *db.Queries, pluginID uuid.UUID, categoryIDs []string) error {
	if err := qtx.PluginCategoriesDeleteByPluginID(ctx, db.PluginCategoriesDeleteByPluginIDParams{
		PluginID: pluginID,
	}); err != nil {
		return writeError(err, "clearing categories")
	}

	for _, categoryID := range categoryIDs {
		if err := qtx.PluginCategoryInsert(ctx, db.PluginCategoryInsertParams{
			PluginID:   pluginID,
			CategoryID: uuid.MustParse(categoryID),
		}); err != nil {
			if isForeignKeyViolation(err) {
				return connect.NewError(connect.CodeInvalidArgument,
					fmt.Errorf("unknown category %s", categoryID))
			}
			return writeError(err, "attaching category")
		}
	}

	return nil
}

func (s *Server) replaceTags(ctx context.Context, qtx *db.Queries, pluginID uuid.UUID, tags []string) error {
	if err := qtx.PluginTagsDeleteByPluginID(ctx, db.PluginTagsDeleteByPluginIDParams{
		PluginID: pluginID,
	}); err != nil {
		return writeError(err, "clearing tags")
	}

	for _, name := range tags {
		// Insert-then-select rather than an upsert: DO UPDATE would need UPDATE
		// on the shared tags vocabulary. Either this call created the row or a
		// committed one already exists, so the select finds it in both cases.
		if err := qtx.TagInsertIfMissing(ctx, db.TagInsertIfMissingParams{Name: name}); err != nil {
			return writeError(err, "creating tag")
		}
		tagID, err := qtx.TagGetByName(ctx, db.TagGetByNameParams{Name: name})
		if err != nil {
			return writeError(err, "resolving tag")
		}
		if err := qtx.PluginTagInsert(ctx, db.PluginTagInsertParams{
			PluginID: pluginID,
			TagID:    tagID,
		}); err != nil {
			return writeError(err, "attaching tag")
		}
	}

	return nil
}

func (s *Server) replaceAllowedOrganizations(ctx context.Context, qtx *db.Queries, pluginID uuid.UUID, organizationIDs []string) error {
	if err := qtx.PluginAllowedOrgsDeleteByPluginID(ctx, db.PluginAllowedOrgsDeleteByPluginIDParams{
		PluginID: pluginID,
	}); err != nil {
		return writeError(err, "clearing allowed organizations")
	}

	for _, organizationID := range organizationIDs {
		if err := qtx.PluginAllowedOrgInsert(ctx, db.PluginAllowedOrgInsertParams{
			PluginID:       pluginID,
			OrganizationID: uuid.MustParse(organizationID),
		}); err != nil {
			if isForeignKeyViolation(err) {
				return connect.NewError(connect.CodeInvalidArgument,
					fmt.Errorf("unknown organization %s", organizationID))
			}
			return writeError(err, "allowing organization")
		}
	}

	return nil
}

// replaceDocumentationLinks applies the request's links to the listing. Unlike
// the other replacements this one is not wholesale: a link is the only child
// row whose id the API hands out, so a link carrying one is updated in place
// and keeps it. The clear runs first and spares those ids, which is also what
// keeps it from soft-deleting the rows this call is about to insert.
func (s *Server) replaceDocumentationLinks(ctx context.Context, qtx *db.Queries, pluginID uuid.UUID, links []*marketplacev1.DocumentationLink) error {
	ids := make([]string, 0, len(links))
	for _, link := range links {
		ids = append(ids, link.GetId())
	}
	kept, err := keptIDs(ids, "documentation link")
	if err != nil {
		return err
	}

	if err := qtx.PluginDocLinksSoftDeleteNotIn(ctx, db.PluginDocLinksSoftDeleteNotInParams{
		PluginID: pluginID,
		KeptIds:  kept,
	}); err != nil {
		return writeError(err, "clearing documentation links")
	}

	for position, link := range links {
		if link.GetId() == "" {
			if err := qtx.PluginDocLinkInsert(ctx, db.PluginDocLinkInsertParams{
				PluginID: pluginID,
				Title:    link.GetTitle(),
				UrlName:  link.GetUrlName(),
				Url:      link.GetUrl(),
				Position: int32(position),
			}); err != nil {
				return writeError(err, "adding documentation link")
			}
			continue
		}

		updated, err := qtx.PluginDocLinkUpdate(ctx, db.PluginDocLinkUpdateParams{
			ID:       uuid.MustParse(link.GetId()),
			PluginID: pluginID,
			Title:    link.GetTitle(),
			UrlName:  link.GetUrlName(),
			Url:      link.GetUrl(),
			Position: int32(position),
		})
		if err != nil {
			return writeError(err, "updating documentation link")
		}
		// Scoped by plugin_id, so this also catches a link id belonging to
		// another listing rather than letting it read as a silent no-op.
		if updated == 0 {
			return connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("unknown documentation link %s", link.GetId()))
		}
	}

	return nil
}

// replaceFeatures is replaceDocumentationLinks for feature blocks: same id rule,
// same ordering, because both are authored child rows whose ids the API hands
// out rather than edges keyed by their content.
func (s *Server) replaceFeatures(ctx context.Context, qtx *db.Queries, pluginID uuid.UUID, features []*marketplacev1.FeatureBlock) error {
	ids := make([]string, 0, len(features))
	for _, feature := range features {
		ids = append(ids, feature.GetId())
	}
	kept, err := keptIDs(ids, "feature block")
	if err != nil {
		return err
	}

	if err := qtx.PluginFeaturesSoftDeleteNotIn(ctx, db.PluginFeaturesSoftDeleteNotInParams{
		PluginID: pluginID,
		KeptIds:  kept,
	}); err != nil {
		return writeError(err, "clearing feature blocks")
	}

	for position, feature := range features {
		if feature.GetId() == "" {
			if err := qtx.PluginFeatureInsert(ctx, db.PluginFeatureInsertParams{
				PluginID: pluginID,
				Title:    feature.GetTitle(),
				Body:     feature.GetBody(),
				Position: int32(position),
			}); err != nil {
				return writeError(err, "adding feature block")
			}
			continue
		}

		updated, err := qtx.PluginFeatureUpdate(ctx, db.PluginFeatureUpdateParams{
			ID:       uuid.MustParse(feature.GetId()),
			PluginID: pluginID,
			Title:    feature.GetTitle(),
			Body:     feature.GetBody(),
			Position: int32(position),
		})
		if err != nil {
			return writeError(err, "updating feature block")
		}
		if updated == 0 {
			return connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("unknown feature block %s", feature.GetId()))
		}
	}

	return nil
}

// keptIDs is the ids a replacement request carries back, in order. A repeat is
// refused: two entries claiming one row would collapse into it, and the caller
// would read back fewer than it sent.
func keptIDs(ids []string, kind string) ([]uuid.UUID, error) {
	kept := make([]uuid.UUID, 0, len(ids))

	for _, id := range ids {
		if id == "" {
			continue
		}
		parsed := uuid.MustParse(id)
		if slices.Contains(kept, parsed) {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("%s %s appears twice", kind, id))
		}
		kept = append(kept, parsed)
	}

	return kept, nil
}

// checkPluginName rejects the two name shapes protovalidate's DNS-1123 rule
// allows but plugins_ck_name does not, which would otherwise surface as a check
// violation rather than a usable error. The double dash is the separator in a
// PluginInstallation's <organization>--<plugin> name (FUN-17); the length is
// because plugins_ck_name spells the first and last character separately.
func checkPluginName(name string) error {
	if len(name) < 2 {
		return fmt.Errorf("plugin name %q must be at least two characters", name)
	}
	if strings.Contains(name, "--") {
		return fmt.Errorf("plugin name %q must not contain a double dash", name)
	}
	return nil
}

// pluginLookupError maps a miss to NOT_FOUND. RLS scopes the role to the
// caller's organization, so a plugin belonging to someone else resolves to
// nothing and is indistinguishable from one that never existed.
func pluginLookupError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return connect.NewError(connect.CodeNotFound, errors.New("plugin not found"))
	}
	return connect.NewError(connect.CodeInternal, fmt.Errorf("loading plugin: %w", err))
}

// writeError maps an RLS rejection to PERMISSION_DENIED. Reaching it means the
// row passed the handler's own lookup but the policy refused the write.
func writeError(err error, action string) error {
	if isRLSDenied(err) {
		return connect.NewError(connect.CodePermissionDenied, errors.New("permission denied"))
	}
	return connect.NewError(connect.CodeInternal, fmt.Errorf("%s: %w", action, err))
}

func isRLSDenied(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.InsufficientPrivilege
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.ForeignKeyViolation
}
