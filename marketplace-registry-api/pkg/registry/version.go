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
	db "github.com/fundament-oss/fundament/marketplace-registry-api/pkg/db/gen"
	registryv1 "github.com/fundament-oss/fundament/marketplace-registry-api/pkg/proto/gen/registry/v1"
)

func (s *Server) ListPluginVersions(
	ctx context.Context,
	req *registryv1.ListPluginVersionsRequest,
) (*registryv1.ListPluginVersionsResponse, error) {
	pluginID := uuid.MustParse(req.GetPluginId())

	if _, err := s.queries.PluginGetByID(ctx, db.PluginGetByIDParams{ID: pluginID}); err != nil {
		return nil, pluginLookupError(err)
	}

	rows, err := s.queries.PluginVersionListByPluginID(ctx, db.PluginVersionListByPluginIDParams{
		PluginID: pluginID,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing plugin versions: %w", err))
	}

	versionIDs := make([]uuid.UUID, 0, len(rows))
	for i := range rows {
		versionIDs = append(versionIDs, rows[i].ID)
	}

	submissions, err := s.loadSubmissions(ctx, versionIDs)
	if err != nil {
		return nil, err
	}

	versions := make([]*registryv1.PluginVersion, 0, len(rows))
	for i := range rows {
		versions = append(versions, versionFromRow(versionRowFromList(&rows[i]), submissions))
	}

	return registryv1.ListPluginVersionsResponse_builder{Versions: versions}.Build(), nil
}

func (s *Server) GetPluginVersion(
	ctx context.Context,
	req *registryv1.GetPluginVersionRequest,
) (*registryv1.GetPluginVersionResponse, error) {
	row, err := s.queries.PluginVersionGetByID(ctx, db.PluginVersionGetByIDParams{
		ID: uuid.MustParse(req.GetPluginVersionId()),
	})
	if err != nil {
		return nil, versionLookupError(err)
	}

	version, err := s.versionFromGetRow(ctx, &row)
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

	plugin, err := s.queries.PluginGetByID(ctx, db.PluginGetByIDParams{ID: pluginID})
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

	// Stored normalized, because plugin_definitions_uq_plugin_version is on the
	// raw text: "v1.2.3" and "1.2.3" would otherwise both land as one plugin
	// claiming the same semantic version twice.
	version := normalizeVersion(req.GetVersion())

	created, err := s.queries.PluginVersionCreate(ctx, db.PluginVersionCreateParams{
		PluginID:      pluginID,
		PluginVersion: version,
		Manifest:      req.GetManifest(),
		Hash:          manifest.hash,
		ReleaseNotes:  req.GetReleaseNotes(),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return nil, connect.NewError(connect.CodeAlreadyExists,
				fmt.Errorf("version %q already exists for this plugin", version))
		}
		return nil, writeError(err, "creating plugin version")
	}

	return registryv1.CreatePluginVersionResponse_builder{
		Version: registryv1.PluginVersion_builder{
			Id:             created.ID.String(),
			PluginId:       pluginID.String(),
			Version:        version,
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

	row, err := s.queries.PluginVersionGetByID(ctx, db.PluginVersionGetByIDParams{ID: versionID})
	if err != nil {
		return nil, versionLookupError(err)
	}

	// Already pending: return the current state rather than opening a second
	// round, which submissions_uq_open would refuse anyway.
	if row.Status == dbconst.PluginDefinitionStatus_Pending {
		version, err := s.versionFromGetRow(ctx, &row)
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

	updated, err := qtx.PluginVersionSetStatus(ctx, db.PluginVersionSetStatusParams{
		ID:             versionID,
		Status:         string(dbconst.PluginDefinitionStatus_Pending),
		ExpectedStatus: string(row.Status),
	})
	if err != nil {
		return nil, writeError(err, "submitting plugin version")
	}
	if updated == 0 {
		return nil, statusChangedError(row.Status)
	}
	if _, err := qtx.SubmissionCreate(ctx, db.SubmissionCreateParams{
		PluginDefinitionID: versionID,
		SubmitterUserID:    userID,
	}); err != nil {
		// submissions_uq_open: a round is open on a version the status says is
		// submittable.
		if isUniqueViolation(err) {
			return nil, connect.NewError(connect.CodeFailedPrecondition,
				errors.New("a submission is already open for this version"))
		}
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

	row, err := s.queries.PluginVersionGetByID(ctx, db.PluginVersionGetByIDParams{ID: versionID})
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

	updated, err := qtx.PluginVersionSetStatus(ctx, db.PluginVersionSetStatusParams{
		ID:             versionID,
		Status:         string(dbconst.PluginDefinitionStatus_Withdrawn),
		ExpectedStatus: string(dbconst.PluginDefinitionStatus_Pending),
	})
	if err != nil {
		return nil, writeError(err, "withdrawing plugin version")
	}
	if updated == 0 {
		return nil, statusChangedError(dbconst.PluginDefinitionStatus_Pending)
	}
	closed, err := qtx.SubmissionCloseOpen(ctx, db.SubmissionCloseOpenParams{
		PluginDefinitionID: versionID,
	})
	if err != nil {
		return nil, writeError(err, "closing submission")
	}
	// The status write above only landed because the version was still PENDING,
	// so a round must have been open. Zero rows means it closed underneath the
	// handler, and withdrawing without closing anything is half a transition.
	if closed == 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			errors.New("no open submission to withdraw"))
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

func versionRowFromGet(row *db.PluginVersionGetByIDRow) *registryVersionRow {
	return &registryVersionRow{
		ID: row.ID, PluginID: row.PluginID, Version: row.PluginVersion,
		Manifest: row.Manifest, Hash: row.Hash, Status: row.Status,
		ReleaseNotes: row.ReleaseNotes, Created: row.Created, Published: row.Published,
	}
}

func versionRowFromList(row *db.PluginVersionListByPluginIDRow) *registryVersionRow {
	return &registryVersionRow{
		ID: row.ID, PluginID: row.PluginID, Version: row.PluginVersion,
		Manifest: row.Manifest, Hash: row.Hash, Status: row.Status,
		ReleaseNotes: row.ReleaseNotes, Created: row.Created, Published: row.Published,
	}
}

// loadSubmissions is the latest review round of each version, keyed by version
// id. A DRAFT has never been submitted, so a version missing from the map is the
// normal case.
func (s *Server) loadSubmissions(
	ctx context.Context,
	versionIDs []uuid.UUID,
) (map[uuid.UUID]db.SubmissionLatestByDefinitionIDsRow, error) {
	submissions := map[uuid.UUID]db.SubmissionLatestByDefinitionIDsRow{}
	if len(versionIDs) == 0 {
		return submissions, nil
	}

	rows, err := s.queries.SubmissionLatestByDefinitionIDs(ctx, db.SubmissionLatestByDefinitionIDsParams{
		PluginDefinitionIds: versionIDs,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("loading submissions: %w", err))
	}
	for _, row := range rows {
		submissions[row.PluginDefinitionID] = row
	}

	return submissions, nil
}

func (s *Server) versionFromGetRow(
	ctx context.Context,
	row *db.PluginVersionGetByIDRow,
) (*registryv1.PluginVersion, error) {
	submissions, err := s.loadSubmissions(ctx, []uuid.UUID{row.ID})
	if err != nil {
		return nil, err
	}
	return versionFromRow(versionRowFromGet(row), submissions), nil
}

func versionFromRow(
	row *registryVersionRow,
	submissions map[uuid.UUID]db.SubmissionLatestByDefinitionIDsRow,
) *registryv1.PluginVersion {
	submission := submissions[row.ID]

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
	}.Build()
}

func (s *Server) loadVersion(ctx context.Context, versionID uuid.UUID) (*registryv1.PluginVersion, error) {
	row, err := s.queries.PluginVersionGetByID(ctx, db.PluginVersionGetByIDParams{ID: versionID})
	if err != nil {
		return nil, versionLookupError(err)
	}
	return s.versionFromGetRow(ctx, &row)
}

// statusChangedError is the zero-row outcome of a guarded status write.
func statusChangedError(expected dbconst.PluginDefinitionStatus) error {
	return connect.NewError(connect.CodeFailedPrecondition,
		fmt.Errorf("plugin version is no longer in status %s", expected))
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
