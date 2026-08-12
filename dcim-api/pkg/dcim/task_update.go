package dcim

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) UpdateTask(
	ctx context.Context,
	req *dcimv1.UpdateTaskRequest,
) (*emptypb.Empty, error) {
	taskID := uuid.MustParse(req.GetId())

	params := db.TaskUpdateParams{
		ID: taskID,
	}

	if req.HasTitle() {
		params.Title = pgtype.Text{String: req.GetTitle(), Valid: true}
	}

	if req.HasStatus() {
		params.Status = pgtype.Text{String: taskStatusFromProto(req.GetStatus()), Valid: true}
	}

	if req.HasPriority() {
		params.Priority = pgtype.Text{String: taskPriorityFromProto(req.GetPriority()), Valid: true}
	}



	// For the nullable columns, an explicitly-set field clears the column when it
	// carries the "empty" sentinel (empty string / epoch timestamp) and otherwise
	// overwrites it. Leaving the field unset keeps the current value.
	//
	// description belongs here too: CreateTask omits a blank one so the column
	// starts NULL, so an edit that empties it has to write NULL as well —
	// otherwise the table ends up with two spellings of "no description", '' on
	// rows that were edited and NULL on rows that never had one.
	if req.HasDescription() {
		if v := req.GetDescription(); v == "" {
			params.ClearDescription = true
		} else {
			params.Description = pgtype.Text{String: v, Valid: true}
		}
	}

	if req.HasAssigneeId() {
		if v := req.GetAssigneeId(); v == "" {
			params.ClearAssignee = true
		} else {
			assigneeID, err := uuid.Parse(v)
			if err != nil {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid assignee_id: %w", err))
			}

			params.AssigneeID = pgtype.UUID{Bytes: assigneeID, Valid: true}
		}
	}

	if req.HasDueDate() {
		if t := req.GetDueDate().AsTime(); t.Equal(time.Unix(0, 0).UTC()) {
			params.ClearDueDate = true
		} else {
			params.DueDate = pgtype.Timestamptz{Time: t, Valid: true}
		}
	}

	if req.HasLocation() {
		if v := req.GetLocation(); v == "" {
			params.ClearLocation = true
		} else {
			params.Location = pgtype.Text{String: v, Valid: true}
		}
	}

	if req.HasBlockedReason() {
		params.BlockedReason = pgtype.Text{String: req.GetBlockedReason(), Valid: true}
	}

	params.ClearBlockedReason = req.GetClearBlockedReason()

	rowsAffected, err := s.queries.TaskUpdate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == dbconst.ConstraintDcimTasksFkAssignee {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("assignee not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update task: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("task not found"))
	}

	// Tags are replaced rather than merged: the request says what the task should
	// carry, so an empty list means it carries none. Leaving the field out
	// entirely leaves the tags alone, which is why this only runs when it is set.
	if req.HasTags() || len(req.GetTags()) > 0 {
		if err := s.queries.TaskTagsClear(ctx, taskID); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to clear task tags: %w", err))
		}

		if tags := req.GetTags(); len(tags) > 0 {
			if err := s.queries.TaskTagsAdd(ctx, db.TaskTagsAddParams{
				TaskID: taskID,
				Tags:   tags,
			}); err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to tag task: %w", err))
			}
		}
	}

	s.logger.InfoContext(ctx, "task updated", "task_id", taskID)

	return &emptypb.Empty{}, nil
}
