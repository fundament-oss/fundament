package registry

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/common/dbconst"
	"github.com/fundament-oss/fundament/common/rollback"
	db "github.com/fundament-oss/fundament/marketplace-api/pkg/db/gen"
	registryv1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/registry/v1"
)

func (s *Server) ListPluginVersions(
	ctx context.Context,
	req *registryv1.ListPluginVersionsRequest,
) (*registryv1.ListPluginVersionsResponse, error) {
	pluginID := uuid.MustParse(req.GetPluginId())

	if _, err := s.queries.RegistryPluginGetByID(ctx, db.RegistryPluginGetByIDParams{ID: pluginID}); err != nil {
		return nil, pluginLookupError(err)
	}

	rows, err := s.queries.RegistryPluginVersionListByPluginID(ctx, db.RegistryPluginVersionListByPluginIDParams{
		PluginID: pluginID,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing plugin versions: %w", err))
	}

	versions := make([]*registryv1.PluginVersion, 0, len(rows))
	for _, row := range rows {
		version, err := s.versionFromRow(ctx, versionRowFromList(row))
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}

	return registryv1.ListPluginVersionsResponse_builder{Versions: versions}.Build(), nil
}

func (s *Server) GetPluginVersion(
	ctx context.Context,
	req *registryv1.GetPluginVersionRequest,
) (*registryv1.GetPluginVersionResponse, error) {
	row, err := s.queries.RegistryPluginVersionGetByID(ctx, db.RegistryPluginVersionGetByIDParams{
		ID: uuid.MustParse(req.GetPluginVersionId()),
	})
	if err != nil {
		return nil, versionLookupError(err)
	}

	version, err := s.versionFromRow(ctx, versionRowFromGet(row))
	if err != nil {
		return nil, err
	}

	return registryv1.GetPluginVersionResponse_builder{Version: version}.Build(), nil
}

// CreatePluginVersion lands a pushed build in DRAFT. No submission is opened:
// pushing is not submitting.
func (s *Server) CreatePluginVersion(
	ctx context.Context,
	req *registryv1.CreatePluginVersionRequest,
) (*registryv1.CreatePluginVersionResponse, error) {
	pluginID := uuid.MustParse(req.GetPluginId())

	plugin, err := s.queries.RegistryPluginGetByID(ctx, db.RegistryPluginGetByIDParams{ID: pluginID})
	if err != nil {
		return nil, pluginLookupError(err)
	}

	manifest, err := parseManifest(req.GetManifest())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if err := manifest.checkMatches(plugin.Name, req.GetVersion()); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	created, err := s.queries.RegistryPluginVersionCreate(ctx, db.RegistryPluginVersionCreateParams{
		PluginID:      pluginID,
		PluginVersion: req.GetVersion(),
		Manifest:      req.GetManifest(),
		Hash:          manifest.hash,
		ReleaseNotes:  req.GetReleaseNotes(),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return nil, connect.NewError(connect.CodeFailedPrecondition,
				fmt.Errorf("version %q already exists for this plugin", req.GetVersion()))
		}
		return nil, writeError(err, "creating plugin version")
	}

	return registryv1.CreatePluginVersionResponse_builder{
		Version: registryv1.PluginVersion_builder{
			Id:             created.ID.String(),
			PluginId:       pluginID.String(),
			Version:        req.GetVersion(),
			Image:          manifest.image,
			DefinitionHash: manifest.hash,
			ReleaseNotes:   req.GetReleaseNotes(),
			Status:         statusFromDB(dbconst.PluginDefinitionStatus_Draft),
			Created:        timestamptzOrNil(created.Created),
		}.Build(),
	}.Build(), nil
}

// SubmitPluginVersion opens a review round. Resubmitting after
// CHANGES_REQUESTED or WITHDRAWN opens a new round rather than reviving the old
// one, so the history of a version stays readable.
func (s *Server) SubmitPluginVersion(
	ctx context.Context,
	req *registryv1.SubmitPluginVersionRequest,
) (*registryv1.SubmitPluginVersionResponse, error) {
	versionID := uuid.MustParse(req.GetPluginVersionId())

	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, errors.New("no user in context"))
	}

	row, err := s.queries.RegistryPluginVersionGetByID(ctx, db.RegistryPluginVersionGetByIDParams{ID: versionID})
	if err != nil {
		return nil, versionLookupError(err)
	}

	// Already pending: return the current state rather than opening a second
	// round, which submissions_uq_open would refuse anyway.
	if row.Status == dbconst.PluginDefinitionStatus_Pending {
		version, err := s.versionFromRow(ctx, versionRowFromGet(row))
		if err != nil {
			return nil, err
		}
		return registryv1.SubmitPluginVersionResponse_builder{Version: version}.Build(), nil
	}

	if !canSubmit(row.Status) {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("cannot submit a version in status %s", row.Status))
	}

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("begin transaction: %w", err))
	}
	defer rollback.Rollback(ctx, tx, s.logger)
	qtx := s.queries.WithTx(tx)

	if err := qtx.RegistryPluginVersionSetStatus(ctx, db.RegistryPluginVersionSetStatusParams{
		ID:     versionID,
		Status: string(dbconst.PluginDefinitionStatus_Pending),
	}); err != nil {
		return nil, writeError(err, "submitting plugin version")
	}
	if _, err := qtx.RegistrySubmissionCreate(ctx, db.RegistrySubmissionCreateParams{
		PluginDefinitionID: versionID,
		SubmitterUserID:    userID,
	}); err != nil {
		return nil, writeError(err, "opening submission")
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("commit transaction: %w", err))
	}

	version, err := s.loadVersion(ctx, versionID)
	if err != nil {
		return nil, err
	}

	return registryv1.SubmitPluginVersionResponse_builder{Version: version}.Build(), nil
}

// WithdrawPluginVersion pulls a pending version back. The round is closed
// without a decision, so reviewed and reviewer_user_id stay null.
func (s *Server) WithdrawPluginVersion(
	ctx context.Context,
	req *registryv1.WithdrawPluginVersionRequest,
) (*registryv1.WithdrawPluginVersionResponse, error) {
	versionID := uuid.MustParse(req.GetPluginVersionId())

	row, err := s.queries.RegistryPluginVersionGetByID(ctx, db.RegistryPluginVersionGetByIDParams{ID: versionID})
	if err != nil {
		return nil, versionLookupError(err)
	}

	if row.Status != dbconst.PluginDefinitionStatus_Pending {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("cannot withdraw a version in status %s", row.Status))
	}

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("begin transaction: %w", err))
	}
	defer rollback.Rollback(ctx, tx, s.logger)
	qtx := s.queries.WithTx(tx)

	if err := qtx.RegistryPluginVersionSetStatus(ctx, db.RegistryPluginVersionSetStatusParams{
		ID:     versionID,
		Status: string(dbconst.PluginDefinitionStatus_Withdrawn),
	}); err != nil {
		return nil, writeError(err, "withdrawing plugin version")
	}
	if err := qtx.RegistrySubmissionCloseOpen(ctx, db.RegistrySubmissionCloseOpenParams{
		PluginDefinitionID: versionID,
	}); err != nil {
		return nil, writeError(err, "closing submission")
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("commit transaction: %w", err))
	}

	version, err := s.loadVersion(ctx, versionID)
	if err != nil {
		return nil, err
	}

	return registryv1.WithdrawPluginVersionResponse_builder{Version: version}.Build(), nil
}

// canSubmit encodes the publisher half of the review lifecycle (FUN-20).
// APPROVED and REJECTED are terminal here: an approved version is immutable
// because its hash is a consent record, and a rejected one is resubmitted as a
// new version rather than revived.
func canSubmit(status dbconst.PluginDefinitionStatus) bool {
	switch status {
	case dbconst.PluginDefinitionStatus_Draft,
		dbconst.PluginDefinitionStatus_ChangesRequested,
		dbconst.PluginDefinitionStatus_Withdrawn:
		return true
	case dbconst.PluginDefinitionStatus_Pending,
		dbconst.PluginDefinitionStatus_Approved,
		dbconst.PluginDefinitionStatus_Rejected:
		return false
	default:
		panic("unhandled PluginDefinitionStatus: " + string(status))
	}
}

// registryVersionRow is the shape the list and get rows share.
type registryVersionRow struct {
	ID           uuid.UUID
	PluginID     uuid.UUID
	Version      string
	Manifest     []byte
	Hash         string
	Status       dbconst.PluginDefinitionStatus
	ReleaseNotes string
	Created      pgtype.Timestamptz
	Published    pgtype.Timestamptz
}

func versionRowFromGet(row db.RegistryPluginVersionGetByIDRow) registryVersionRow {
	return registryVersionRow{
		ID: row.ID, PluginID: row.PluginID, Version: row.PluginVersion,
		Manifest: row.Manifest, Hash: row.Hash, Status: row.Status,
		ReleaseNotes: row.ReleaseNotes, Created: row.Created, Published: row.Published,
	}
}

func versionRowFromList(row db.RegistryPluginVersionListByPluginIDRow) registryVersionRow {
	return registryVersionRow{
		ID: row.ID, PluginID: row.PluginID, Version: row.PluginVersion,
		Manifest: row.Manifest, Hash: row.Hash, Status: row.Status,
		ReleaseNotes: row.ReleaseNotes, Created: row.Created, Published: row.Published,
	}
}

func (s *Server) versionFromRow(ctx context.Context, row registryVersionRow) (*registryv1.PluginVersion, error) {
	// A DRAFT has never been submitted, so no row here is the normal case.
	submission, err := s.queries.RegistrySubmissionLatestByDefinitionID(ctx, db.RegistrySubmissionLatestByDefinitionIDParams{
		PluginDefinitionID: row.ID,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("loading submission: %w", err))
	}

	return registryv1.PluginVersion_builder{
		Id:             row.ID.String(),
		PluginId:       row.PluginID.String(),
		Version:        row.Version,
		Image:          imageFor(row.Manifest),
		DefinitionHash: row.Hash,
		ReleaseNotes:   row.ReleaseNotes,
		Status:         statusFromDB(row.Status),
		Created:        timestamptzOrNil(row.Created),
		Submitted:      timestamptzOrNil(submission.Submitted),
		Published:      timestamptzOrNil(row.Published),
		ReviewFeedback: submission.Feedback,
	}.Build(), nil
}

func (s *Server) loadVersion(ctx context.Context, versionID uuid.UUID) (*registryv1.PluginVersion, error) {
	row, err := s.queries.RegistryPluginVersionGetByID(ctx, db.RegistryPluginVersionGetByIDParams{ID: versionID})
	if err != nil {
		return nil, versionLookupError(err)
	}
	return s.versionFromRow(ctx, versionRowFromGet(row))
}

// versionLookupError mirrors pluginLookupError: RLS reaches the owning
// organization through the version's plugin, so someone else's version and a
// nonexistent one are the same miss.
func versionLookupError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return connect.NewError(connect.CodeNotFound, errors.New("plugin version not found"))
	}
	return connect.NewError(connect.CodeInternal, fmt.Errorf("loading plugin version: %w", err))
}
