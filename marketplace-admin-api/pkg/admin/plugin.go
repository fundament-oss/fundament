package admin

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/marketplace-admin-api/pkg/db/gen"
	adminv1 "github.com/fundament-oss/fundament/marketplace-admin-api/pkg/proto/gen/admin/v1"
	marketplacev1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/marketplace/v1"
)

// ListPlugins resolves every listing in the queue in one call, which is what
// it exists for: a GetPlugin per submission would be an N+1 over the whole
// queue.
func (s *Server) ListPlugins(
	ctx context.Context,
	_ *adminv1.ListPluginsRequest,
) (*adminv1.ListPluginsResponse, error) {
	rows, err := s.queries.PluginList(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing plugins: %w", err))
	}

	pluginIDs := make([]uuid.UUID, 0, len(rows))
	for i := range rows {
		pluginIDs = append(pluginIDs, rows[i].ID)
	}

	tags, categories, err := s.loadPluginChildren(ctx, pluginIDs)
	if err != nil {
		return nil, err
	}

	plugins := make([]*adminv1.Plugin, 0, len(rows))
	for i := range rows {
		plugins = append(plugins, pluginFromRow(pluginRowFromList(&rows[i]), tags, categories))
	}

	return adminv1.ListPluginsResponse_builder{Plugins: plugins}.Build(), nil
}

func (s *Server) GetPlugin(
	ctx context.Context,
	req *adminv1.GetPluginRequest,
) (*adminv1.GetPluginResponse, error) {
	row, err := s.queries.PluginGetByID(ctx, db.PluginGetByIDParams{
		ID: uuid.MustParse(req.GetPluginId()),
	})
	if err != nil {
		return nil, pluginLookupError(err)
	}

	tags, categories, err := s.loadPluginChildren(ctx, []uuid.UUID{row.ID})
	if err != nil {
		return nil, err
	}

	return adminv1.GetPluginResponse_builder{
		Plugin: pluginFromRow(pluginRowFromGet(&row), tags, categories),
	}.Build(), nil
}

// GetPluginVersion carries the review payload — what the plugin will be
// allowed to do once installed — derived from the version's pinned manifest.
func (s *Server) GetPluginVersion(
	ctx context.Context,
	req *adminv1.GetPluginVersionRequest,
) (*adminv1.GetPluginVersionResponse, error) {
	row, err := s.queries.PluginVersionGetByID(ctx, db.PluginVersionGetByIDParams{
		ID: uuid.MustParse(req.GetPluginVersionId()),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("plugin version not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("loading plugin version: %w", err))
	}

	capabilities, permissions, err := capabilitiesAndPermissions(row.Manifest)
	if err != nil {
		// The registry validated these bytes at push, so a parse failure is a
		// data problem, not a request problem. The version still renders — a
		// reviewer confronted with it should reject, not get a 500.
		s.logger.WarnContext(ctx, "failed to derive capabilities from manifest",
			"plugin_version_id", row.ID, "error", err)
		capabilities, permissions = nil, nil
	}

	return adminv1.GetPluginVersionResponse_builder{
		Version: adminv1.PluginVersion_builder{
			Id:             row.ID.String(),
			PluginId:       row.PluginID.String(),
			Version:        row.PluginVersion,
			Image:          row.Image,
			DefinitionHash: row.Hash,
			ReleaseNotes:   row.ReleaseNotes,
			Status:         statusFromDB(row.Status),
			Capabilities:   capabilities,
			Permissions:    permissions,
			Created:        timestamptzOrNil(row.Created),
			Submitted:      timestamptzOrNil(row.Submitted),
			Published:      timestamptzOrNil(row.Published),
		}.Build(),
	}.Build(), nil
}

func (s *Server) ListCategories(
	ctx context.Context,
	_ *adminv1.ListCategoriesRequest,
) (*adminv1.ListCategoriesResponse, error) {
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

	return adminv1.ListCategoriesResponse_builder{Categories: categories}.Build(), nil
}

// ListPublishers serves the publishing organizations. organization-api cannot:
// its GetOrganization is RLS-scoped to members, and a reviewer is not a member
// of the organization under review.
func (s *Server) ListPublishers(
	ctx context.Context,
	_ *adminv1.ListPublishersRequest,
) (*adminv1.ListPublishersResponse, error) {
	rows, err := s.queries.PublisherList(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing publishers: %w", err))
	}

	publishers := make([]*marketplacev1.Publisher, 0, len(rows))
	for _, row := range rows {
		publishers = append(publishers, marketplacev1.Publisher_builder{
			Id:          row.ID.String(),
			Name:        row.Name,
			DisplayName: row.Alias,
		}.Build())
	}

	return adminv1.ListPublishersResponse_builder{Publishers: publishers}.Build(), nil
}

// adminPluginRow is the shape the list and get rows share.
type adminPluginRow struct {
	ID               uuid.UUID
	Name             string
	DisplayName      string
	DescriptionShort string
	Description      string
	OrganizationID   uuid.UUID
	Image            string
	Created          pgtype.Timestamptz
	Updated          pgtype.Timestamptz
}

func pluginRowFromList(row *db.PluginListRow) *adminPluginRow {
	return &adminPluginRow{
		ID: row.ID, Name: row.Name, DisplayName: row.DisplayName,
		DescriptionShort: row.DescriptionShort, Description: row.Description,
		OrganizationID: row.OrganizationID, Image: row.Image,
		Created: row.Created, Updated: row.Updated,
	}
}

func pluginRowFromGet(row *db.PluginGetByIDRow) *adminPluginRow {
	return &adminPluginRow{
		ID: row.ID, Name: row.Name, DisplayName: row.DisplayName,
		DescriptionShort: row.DescriptionShort, Description: row.Description,
		OrganizationID: row.OrganizationID, Image: row.Image,
		Created: row.Created, Updated: row.Updated,
	}
}

func pluginFromRow(
	row *adminPluginRow,
	tags map[uuid.UUID][]string,
	categories map[uuid.UUID][]string,
) *adminv1.Plugin {
	return adminv1.Plugin_builder{
		Id:               row.ID.String(),
		Name:             row.Name,
		DisplayName:      row.DisplayName,
		DescriptionShort: row.DescriptionShort,
		Description:      row.Description,
		OrganizationId:   row.OrganizationID.String(),
		Image:            row.Image,
		CategoryIds:      categories[row.ID],
		Tags:             tags[row.ID],
		Created:          timestamptzOrNil(row.Created),
		Updated:          timestamptzOrNil(row.Updated),
	}.Build()
}

// loadPluginChildren reads every listing's tags and category ids in two
// queries rather than two per listing.
func (s *Server) loadPluginChildren(
	ctx context.Context,
	pluginIDs []uuid.UUID,
) (tags, categories map[uuid.UUID][]string, err error) {
	tags = map[uuid.UUID][]string{}
	categories = map[uuid.UUID][]string{}
	if len(pluginIDs) == 0 {
		return tags, categories, nil
	}

	tagRows, err := s.queries.PluginTagsListByPluginIDs(ctx, db.PluginTagsListByPluginIDsParams{
		PluginIds: pluginIDs,
	})
	if err != nil {
		return nil, nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing plugin tags: %w", err))
	}
	for _, row := range tagRows {
		tags[row.PluginID] = append(tags[row.PluginID], row.Name)
	}

	categoryRows, err := s.queries.PluginCategoriesListByPluginIDs(ctx, db.PluginCategoriesListByPluginIDsParams{
		PluginIds: pluginIDs,
	})
	if err != nil {
		return nil, nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing plugin categories: %w", err))
	}
	for _, row := range categoryRows {
		categories[row.PluginID] = append(categories[row.PluginID], row.CategoryID.String())
	}

	return tags, categories, nil
}

// pluginLookupError maps a miss to NOT_FOUND. Unlike the registry's, there is
// no tenancy dimension here: the admin role sees every listing, so a miss
// really is a listing that does not exist (or was soft-deleted).
func pluginLookupError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return connect.NewError(connect.CodeNotFound, errors.New("plugin not found"))
	}
	return connect.NewError(connect.CodeInternal, fmt.Errorf("loading plugin: %w", err))
}

// writeError maps an RLS or grant rejection to PERMISSION_DENIED. Reaching it
// means the row passed the handler's own lookup but the database refused the
// write — including a column outside the admin role's fenced UPDATE grants, so
// the pg detail is logged before the response goes opaque.
func (s *Server) writeError(ctx context.Context, err error, action string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.InsufficientPrivilege {
		s.logger.WarnContext(ctx, "write refused with insufficient_privilege",
			"action", action,
			"message", pgErr.Message,
			"table", pgErr.TableName,
			"constraint", pgErr.ConstraintName,
		)
		return connect.NewError(connect.CodePermissionDenied, errors.New("permission denied"))
	}
	return connect.NewError(connect.CodeInternal, fmt.Errorf("%s: %w", action, err))
}
