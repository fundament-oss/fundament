package dcim

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) CreateTask(
	ctx context.Context,
	req *dcimv1.CreateTaskRequest,
) (*dcimv1.CreateTaskResponse, error) {
	params := db.TaskCreateParams{
		Title:    req.GetTitle(),
		Status:   taskStatusFromProto(req.GetStatus()),
		Priority: taskPriorityFromProto(req.GetPriority()),
	}

	if req.HasBlockedReason() {
		params.BlockedReason = pgtype.Text{String: req.GetBlockedReason(), Valid: true}
	}

	if req.HasDescription() {
		params.Description = pgtype.Text{String: req.GetDescription(), Valid: true}
	}

	if req.HasAssigneeId() {
		assigneeID, err := uuid.Parse(req.GetAssigneeId())
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid assignee_id: %w", err))
		}

		params.AssigneeID = pgtype.UUID{Bytes: assigneeID, Valid: true}
	}

	if req.HasDueDate() {
		params.DueDate = pgtype.Timestamptz{Time: req.GetDueDate().AsTime(), Valid: true}
	}

	if req.HasLocation() {
		params.Location = pgtype.Text{String: req.GetLocation(), Valid: true}
	}

	id, err := s.queries.TaskCreate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == dbconst.ConstraintDcimTasksFkAssignee {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("assignee not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create task: %w", err))
	}

	// The tags live in their own table, so they are written after the task
	// exists. A task without tags is a normal task, so an empty list is not an
	// error and simply writes nothing.
	if tags := req.GetTags(); len(tags) > 0 {
		if err := s.queries.TaskTagsAdd(ctx, db.TaskTagsAddParams{
			TaskID: id,
			Tags:   tags,
		}); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to tag task: %w", err))
		}
	}

	s.logger.InfoContext(ctx, "task created", "task_id", id)

	return dcimv1.CreateTaskResponse_builder{
		TaskId: id.String(),
	}.Build(), nil
}
