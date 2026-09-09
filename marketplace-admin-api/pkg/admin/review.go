package admin

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
	db "github.com/fundament-oss/fundament/marketplace-admin-api/pkg/db/gen"
	adminv1 "github.com/fundament-oss/fundament/marketplace-admin-api/pkg/proto/gen/admin/v1"
)

func (s *Server) ListSubmissions(
	ctx context.Context,
	req *adminv1.ListSubmissionsRequest,
) (*adminv1.ListSubmissionsResponse, error) {
	rows, err := s.queries.SubmissionList(ctx, db.SubmissionListParams{
		Status: statusFilterToDB(req.GetStatus()),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("listing submissions: %w", err))
	}

	submissions := make([]*adminv1.Submission, 0, len(rows))
	for i := range rows {
		submissions = append(submissions, submissionFromRow(submissionRowFromList(&rows[i])))
	}

	return adminv1.ListSubmissionsResponse_builder{Submissions: submissions}.Build(), nil
}

func (s *Server) GetSubmission(
	ctx context.Context,
	req *adminv1.GetSubmissionRequest,
) (*adminv1.GetSubmissionResponse, error) {
	row, err := s.queries.SubmissionGetByID(ctx, db.SubmissionGetByIDParams{
		ID: uuid.MustParse(req.GetSubmissionId()),
	})
	if err != nil {
		return nil, submissionLookupError(err)
	}

	return adminv1.GetSubmissionResponse_builder{
		Submission: submissionFromRow(submissionRowFromGet(&row)),
	}.Build(), nil
}

// ApproveSubmission publishes the version. Approving a plugin's first version
// makes the listing live in the public catalog: publication is nothing more
// than the published timestamp this write stamps.
func (s *Server) ApproveSubmission(
	ctx context.Context,
	req *adminv1.ApproveSubmissionRequest,
) (*adminv1.ApproveSubmissionResponse, error) {
	submission, err := s.decide(ctx, uuid.MustParse(req.GetSubmissionId()), decision{
		status:   dbconst.PluginDefinitionStatus_Approved,
		feedback: req.GetFeedback(),
	})
	if err != nil {
		return nil, err
	}
	return adminv1.ApproveSubmissionResponse_builder{Submission: submission}.Build(), nil
}

// RequestChanges closes the round with feedback; the developer may resubmit,
// which opens a fresh round rather than reviving this one.
func (s *Server) RequestChanges(
	ctx context.Context,
	req *adminv1.RequestChangesRequest,
) (*adminv1.RequestChangesResponse, error) {
	submission, err := s.decide(ctx, uuid.MustParse(req.GetSubmissionId()), decision{
		status:   dbconst.PluginDefinitionStatus_ChangesRequested,
		feedback: req.GetFeedback(),
	})
	if err != nil {
		return nil, err
	}
	return adminv1.RequestChangesResponse_builder{Submission: submission}.Build(), nil
}

// RejectSubmission refuses the version outright. Unlike RequestChanges this is
// terminal on the publisher surface: a rejected version is resubmitted as a
// new version rather than revived.
func (s *Server) RejectSubmission(
	ctx context.Context,
	req *adminv1.RejectSubmissionRequest,
) (*adminv1.RejectSubmissionResponse, error) {
	submission, err := s.decide(ctx, uuid.MustParse(req.GetSubmissionId()), decision{
		status:          dbconst.PluginDefinitionStatus_Rejected,
		rejectionReason: rejectionReasonToDB(req.GetReason()),
		feedback:        req.GetFeedback(),
	})
	if err != nil {
		return nil, err
	}
	return adminv1.RejectSubmissionResponse_builder{Submission: submission}.Build(), nil
}

// decision is the reviewer's verdict: the status the version moves to, and
// what gets recorded on the round.
type decision struct {
	status          dbconst.PluginDefinitionStatus
	rejectionReason string
	feedback        string
}

// decide runs the shared half of every review verdict: only a PENDING version
// with an open round can be decided, the version's status and the round's
// record move in one transaction, and both writes are guarded so a decision
// racing a withdrawal (or another reviewer) fails rather than half-landing.
func (s *Server) decide(ctx context.Context, submissionID uuid.UUID, d decision) (*adminv1.Submission, error) {
	reviewerID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, errors.New("no user in context"))
	}

	row, err := s.queries.SubmissionGetByID(ctx, db.SubmissionGetByIDParams{ID: submissionID})
	if err != nil {
		return nil, submissionLookupError(err)
	}

	if row.Status != dbconst.PluginDefinitionStatus_Pending {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("cannot decide a submission in status %s", row.Status))
	}
	// The version is PENDING, but this round may be a closed one from an
	// earlier submit: deciding it would record the verdict on history.
	if row.Closed.Valid {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			errors.New("submission round is closed"))
	}

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("begin transaction: %w", err))
	}
	defer rollback.Rollback(ctx, tx, s.logger)
	qtx := s.queries.WithTx(tx)

	var updated int64
	if d.status == dbconst.PluginDefinitionStatus_Approved {
		updated, err = qtx.PluginVersionApprove(ctx, db.PluginVersionApproveParams{
			ID: row.PluginDefinitionID,
		})
	} else {
		updated, err = qtx.PluginVersionSetStatus(ctx, db.PluginVersionSetStatusParams{
			ID:     row.PluginDefinitionID,
			Status: string(d.status),
		})
	}
	if err != nil {
		return nil, s.writeError(ctx, err, "updating version status")
	}
	if updated == 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			errors.New("plugin version is no longer in status pending"))
	}

	rejectionReason := pgtype.Text{}
	if d.rejectionReason != "" {
		rejectionReason = pgtype.Text{String: d.rejectionReason, Valid: true}
	}
	closed, err := qtx.SubmissionDecide(ctx, db.SubmissionDecideParams{
		ID:              submissionID,
		ReviewerUserID:  reviewerID,
		RejectionReason: rejectionReason,
		Feedback:        d.feedback,
	})
	if err != nil {
		return nil, s.writeError(ctx, err, "recording decision")
	}
	// The status write above only landed because the version was still PENDING,
	// so this round must have been open. Zero rows means it closed underneath
	// the handler, and reporting a decision that recorded nothing is half a
	// transition.
	if closed == 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			errors.New("submission round is closed"))
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("commit transaction: %w", err))
	}

	decided, err := s.queries.SubmissionGetByID(ctx, db.SubmissionGetByIDParams{ID: submissionID})
	if err != nil {
		return nil, submissionLookupError(err)
	}
	return submissionFromRow(submissionRowFromGet(&decided)), nil
}

// adminSubmissionRow is the shape the list and get rows share.
type adminSubmissionRow struct {
	ID                 uuid.UUID
	PluginID           uuid.UUID
	PluginDefinitionID uuid.UUID
	OrganizationID     uuid.UUID
	SubmitterUserID    uuid.UUID
	Status             dbconst.PluginDefinitionStatus
	Submitted          pgtype.Timestamptz
	Reviewed           pgtype.Timestamptz
	Closed             pgtype.Timestamptz
	ReviewerUserID     pgtype.UUID
	RejectionReason    pgtype.Text
	Feedback           string
}

func submissionRowFromList(row *db.SubmissionListRow) *adminSubmissionRow {
	return &adminSubmissionRow{
		ID: row.ID, PluginID: row.PluginID, PluginDefinitionID: row.PluginDefinitionID,
		OrganizationID: row.OrganizationID, SubmitterUserID: row.SubmitterUserID,
		Status: row.Status, Submitted: row.Submitted, Reviewed: row.Reviewed,
		Closed: row.Closed, ReviewerUserID: row.ReviewerUserID, RejectionReason: row.RejectionReason,
		Feedback: row.Feedback,
	}
}

func submissionRowFromGet(row *db.SubmissionGetByIDRow) *adminSubmissionRow {
	return &adminSubmissionRow{
		ID: row.ID, PluginID: row.PluginID, PluginDefinitionID: row.PluginDefinitionID,
		OrganizationID: row.OrganizationID, SubmitterUserID: row.SubmitterUserID,
		Status: row.Status, Submitted: row.Submitted, Reviewed: row.Reviewed,
		Closed: row.Closed, ReviewerUserID: row.ReviewerUserID, RejectionReason: row.RejectionReason,
		Feedback: row.Feedback,
	}
}

func submissionFromRow(row *adminSubmissionRow) *adminv1.Submission {
	return adminv1.Submission_builder{
		Id:              row.ID.String(),
		PluginId:        row.PluginID.String(),
		PluginVersionId: row.PluginDefinitionID.String(),
		OrganizationId:  row.OrganizationID.String(),
		SubmitterUserId: row.SubmitterUserID.String(),
		Status:          statusFromDB(row.Status),
		Submitted:       timestamptzOrNil(row.Submitted),
		Reviewed:        timestamptzOrNil(row.Reviewed),
		ReviewerUserId:  pgUUIDOrEmpty(row.ReviewerUserID),
		RejectionReason: rejectionReasonFromDB(row.RejectionReason),
		Feedback:        row.Feedback,
	}.Build()
}

func submissionLookupError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return connect.NewError(connect.CodeNotFound, errors.New("submission not found"))
	}
	return connect.NewError(connect.CodeInternal, fmt.Errorf("loading submission: %w", err))
}
