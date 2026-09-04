package registry

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/rollback"
	db "github.com/fundament-oss/fundament/marketplace-api/pkg/db/gen"
	marketplacev1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/marketplace/v1"
	registryv1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/registry/v1"
)

func (s *Server) ListPlugins(
	ctx context.Context,
	_ *registryv1.ListPluginsRequest,
) (*registryv1.ListPluginsResponse, error) {
	rows, err := s.queries.RegistryPluginList(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing plugins: %w", err))
	}

	plugins := make([]*registryv1.Plugin, 0, len(rows))
	for _, row := range rows {
		plugin, err := s.pluginFromRow(ctx, pluginRowFromList(row))
		if err != nil {
			return nil, err
		}
		plugins = append(plugins, plugin)
	}

	return registryv1.ListPluginsResponse_builder{Plugins: plugins}.Build(), nil
}

func (s *Server) GetPlugin(
	ctx context.Context,
	req *registryv1.GetPluginRequest,
) (*registryv1.GetPluginResponse, error) {
	row, err := s.queries.RegistryPluginGetByID(ctx, db.RegistryPluginGetByIDParams{
		ID: uuid.MustParse(req.GetPluginId()),
	})
	if err != nil {
		return nil, pluginLookupError(err)
	}

	plugin, err := s.pluginFromRow(ctx, pluginRowFromGet(row))
	if err != nil {
		return nil, err
	}

	return registryv1.GetPluginResponse_builder{Plugin: plugin}.Build(), nil
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

	pluginID, err := qtx.RegistryPluginCreate(ctx, db.RegistryPluginCreateParams{
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
	if _, err := s.queries.RegistryPluginGetByID(ctx, db.RegistryPluginGetByIDParams{ID: pluginID}); err != nil {
		return nil, pluginLookupError(err)
	}

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("begin transaction: %w", err))
	}
	defer rollback.Rollback(ctx, tx, s.logger)
	qtx := s.queries.WithTx(tx)

	if err := qtx.RegistryPluginUpdate(ctx, db.RegistryPluginUpdateParams{
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
	}); err != nil {
		return nil, writeError(err, "updating plugin")
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

	if _, err := s.queries.RegistryPluginGetByID(ctx, db.RegistryPluginGetByIDParams{ID: pluginID}); err != nil {
		return nil, pluginLookupError(err)
	}

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("begin transaction: %w", err))
	}
	defer rollback.Rollback(ctx, tx, s.logger)
	qtx := s.queries.WithTx(tx)

	if err := qtx.RegistrySubmissionCloseOpenByPluginID(ctx, db.RegistrySubmissionCloseOpenByPluginIDParams{
		PluginID: pluginID,
	}); err != nil {
		return nil, writeError(err, "closing open submissions")
	}
	if err := qtx.RegistryPluginVersionsSoftDeleteByPluginID(ctx, db.RegistryPluginVersionsSoftDeleteByPluginIDParams{
		PluginID: pluginID,
	}); err != nil {
		return nil, writeError(err, "deleting plugin versions")
	}
	if err := qtx.RegistryPluginSoftDelete(ctx, db.RegistryPluginSoftDeleteParams{ID: pluginID}); err != nil {
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
	rows, err := s.queries.RegistryCategoryList(ctx)
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

func pluginRowFromList(row db.RegistryPluginListRow) registryPluginRow {
	return registryPluginRow{
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

func pluginRowFromGet(row db.RegistryPluginGetByIDRow) registryPluginRow {
	return registryPluginRow{
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
	row, err := s.queries.RegistryPluginGetByID(ctx, db.RegistryPluginGetByIDParams{ID: pluginID})
	if err != nil {
		return nil, pluginLookupError(err)
	}
	return s.pluginFromRow(ctx, pluginRowFromGet(row))
}

func (s *Server) pluginFromRow(ctx context.Context, row registryPluginRow) (*registryv1.Plugin, error) {
	categoryIDs, err := s.queries.RegistryPluginCategoriesListByPluginID(ctx, db.RegistryPluginCategoriesListByPluginIDParams{
		PluginID: row.ID,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing plugin categories: %w", err))
	}

	tags, err := s.queries.RegistryPluginTagsListByPluginID(ctx, db.RegistryPluginTagsListByPluginIDParams{
		PluginID: row.ID,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing plugin tags: %w", err))
	}

	allowedOrgs, err := s.queries.RegistryPluginAllowedOrgsListByPluginID(ctx, db.RegistryPluginAllowedOrgsListByPluginIDParams{
		PluginID: row.ID,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing allowed organizations: %w", err))
	}

	linkRows, err := s.queries.RegistryPluginDocLinksListByPluginID(ctx, db.RegistryPluginDocLinksListByPluginIDParams{
		PluginID: row.ID,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing documentation links: %w", err))
	}

	featureRows, err := s.queries.RegistryPluginFeaturesListByPluginID(ctx, db.RegistryPluginFeaturesListByPluginIDParams{
		PluginID: row.ID,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing feature blocks: %w", err))
	}

	links := make([]*marketplacev1.DocumentationLink, 0, len(linkRows))
	for _, link := range linkRows {
		links = append(links, marketplacev1.DocumentationLink_builder{
			Id:      link.ID.String(),
			Title:   link.Title,
			UrlName: link.UrlName,
			Url:     link.Url,
		}.Build())
	}

	features := make([]*marketplacev1.FeatureBlock, 0, len(featureRows))
	for _, feature := range featureRows {
		features = append(features, marketplacev1.FeatureBlock_builder{
			Title: feature.Title,
			Body:  feature.Body,
		}.Build())
	}

	allowed := make([]string, 0, len(allowedOrgs))
	for _, id := range allowedOrgs {
		allowed = append(allowed, id.String())
	}

	categories := make([]string, 0, len(categoryIDs))
	for _, id := range categoryIDs {
		categories = append(categories, id.String())
	}

	return registryv1.Plugin_builder{
		Id:                       row.ID.String(),
		Name:                     row.Name,
		DisplayName:              row.DisplayName,
		DescriptionShort:         row.DescriptionShort,
		Description:              row.Description,
		OrganizationId:           row.OrganizationID.String(),
		Image:                    row.Image,
		CategoryIds:              categories,
		Tags:                     tags,
		AuthorName:               row.AuthorName,
		AuthorUrl:                row.AuthorURL,
		RepositoryUrl:            row.RepositoryURL,
		License:                  row.License,
		DocumentationLinks:       links,
		Features:                 features,
		Visibility:               row.Visibility,
		AllowedOrganizationIds:   allowed,
		LatestPublishedVersionId: uuidOrEmpty(row.LatestPublishedVersionID),
		Created:                  row.Created,
		Updated:                  row.Updated,
	}.Build(), nil
}

func (s *Server) replaceCategories(ctx context.Context, qtx *db.Queries, pluginID uuid.UUID, categoryIDs []string) error {
	if err := qtx.RegistryPluginCategoriesDeleteByPluginID(ctx, db.RegistryPluginCategoriesDeleteByPluginIDParams{
		PluginID: pluginID,
	}); err != nil {
		return writeError(err, "clearing categories")
	}

	for _, categoryID := range categoryIDs {
		if err := qtx.RegistryPluginCategoryInsert(ctx, db.RegistryPluginCategoryInsertParams{
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
	if err := qtx.RegistryPluginTagsDeleteByPluginID(ctx, db.RegistryPluginTagsDeleteByPluginIDParams{
		PluginID: pluginID,
	}); err != nil {
		return writeError(err, "clearing tags")
	}

	for _, name := range tags {
		// Insert-then-select rather than an upsert: DO UPDATE would need UPDATE
		// on the shared tags vocabulary. Either this call created the row or a
		// committed one already exists, so the select finds it in both cases.
		if err := qtx.RegistryTagInsertIfMissing(ctx, db.RegistryTagInsertIfMissingParams{Name: name}); err != nil {
			return writeError(err, "creating tag")
		}
		tagID, err := qtx.RegistryTagGetByName(ctx, db.RegistryTagGetByNameParams{Name: name})
		if err != nil {
			return writeError(err, "resolving tag")
		}
		if err := qtx.RegistryPluginTagInsert(ctx, db.RegistryPluginTagInsertParams{
			PluginID: pluginID,
			TagID:    tagID,
		}); err != nil {
			return writeError(err, "attaching tag")
		}
	}

	return nil
}

func (s *Server) replaceAllowedOrganizations(ctx context.Context, qtx *db.Queries, pluginID uuid.UUID, organizationIDs []string) error {
	if err := qtx.RegistryPluginAllowedOrgsDeleteByPluginID(ctx, db.RegistryPluginAllowedOrgsDeleteByPluginIDParams{
		PluginID: pluginID,
	}); err != nil {
		return writeError(err, "clearing allowed organizations")
	}

	for _, organizationID := range organizationIDs {
		if err := qtx.RegistryPluginAllowedOrgInsert(ctx, db.RegistryPluginAllowedOrgInsertParams{
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

func (s *Server) replaceDocumentationLinks(ctx context.Context, qtx *db.Queries, pluginID uuid.UUID, links []*marketplacev1.DocumentationLink) error {
	if err := qtx.RegistryPluginDocLinksSoftDeleteByPluginID(ctx, db.RegistryPluginDocLinksSoftDeleteByPluginIDParams{
		PluginID: pluginID,
	}); err != nil {
		return writeError(err, "clearing documentation links")
	}

	for position, link := range links {
		if err := qtx.RegistryPluginDocLinkInsert(ctx, db.RegistryPluginDocLinkInsertParams{
			PluginID: pluginID,
			Title:    link.GetTitle(),
			UrlName:  link.GetUrlName(),
			Url:      link.GetUrl(),
			Position: int32(position),
		}); err != nil {
			return writeError(err, "adding documentation link")
		}
	}

	return nil
}

func (s *Server) replaceFeatures(ctx context.Context, qtx *db.Queries, pluginID uuid.UUID, features []*marketplacev1.FeatureBlock) error {
	if err := qtx.RegistryPluginFeaturesSoftDeleteByPluginID(ctx, db.RegistryPluginFeaturesSoftDeleteByPluginIDParams{
		PluginID: pluginID,
	}); err != nil {
		return writeError(err, "clearing feature blocks")
	}

	for position, feature := range features {
		if err := qtx.RegistryPluginFeatureInsert(ctx, db.RegistryPluginFeatureInsertParams{
			PluginID: pluginID,
			Title:    feature.GetTitle(),
			Body:     feature.GetBody(),
			Position: int32(position),
		}); err != nil {
			return writeError(err, "adding feature block")
		}
	}

	return nil
}

// checkPluginName rejects the one name shape protovalidate's DNS-1123 rule
// allows but plugins_ck_name does not. The double dash is the separator in a
// PluginInstallation's <organization>--<plugin> name (FUN-17), so letting it
// through would surface as a check violation rather than a usable error.
func checkPluginName(name string) error {
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
