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
	req *connect.Request[dcimv1.UpdateTaskRequest],
) (*connect.Response[emptypb.Empty], error) {
	taskID := uuid.MustParse(req.Msg.GetId())

	params := db.TaskUpdateParams{
		ID: taskID,
	}

	if req.Msg.HasTitle() {
		params.Title = pgtype.Text{String: req.Msg.GetTitle(), Valid: true}
	}

	if req.Msg.HasStatus() {
		params.Status = pgtype.Text{String: taskStatusFromProto(req.Msg.GetStatus()), Valid: true}
	}

	if req.Msg.HasPriority() {
		params.Priority = pgtype.Text{String: taskPriorityFromProto(req.Msg.GetPriority()), Valid: true}
	}

	if req.Msg.HasCategory() {
		params.Category = pgtype.Text{String: taskCategoryFromProto(req.Msg.GetCategory()), Valid: true}
	}

	// For the nullable columns, an explicitly-set field clears the column when it
	// carries the "empty" sentinel (empty string / epoch timestamp) and otherwise
	// overwrites it. Leaving the field unset keeps the current value.
	//
	// description belongs here too: CreateTask omits a blank one so the column
	// starts NULL, so an edit that empties it has to write NULL as well —
	// otherwise the table ends up with two spellings of "no description", '' on
	// rows that were edited and NULL on rows that never had one.
	if req.Msg.HasDescription() {
		if v := req.Msg.GetDescription(); v == "" {
			params.ClearDescription = true
		} else {
			params.Description = pgtype.Text{String: v, Valid: true}
		}
	}

	if req.Msg.HasAssigneeId() {
		if v := req.Msg.GetAssigneeId(); v == "" {
			params.ClearAssignee = true
		} else {
			assigneeID, err := uuid.Parse(v)
			if err != nil {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid assignee_id: %w", err))
			}

			params.AssigneeID = pgtype.UUID{Bytes: assigneeID, Valid: true}
		}
	}

	if req.Msg.HasDueDate() {
		if t := req.Msg.GetDueDate().AsTime(); t.Equal(time.Unix(0, 0).UTC()) {
			params.ClearDueDate = true
		} else {
			params.DueDate = pgtype.Timestamptz{Time: t, Valid: true}
		}
	}

	if req.Msg.HasLocation() {
		if v := req.Msg.GetLocation(); v == "" {
			params.ClearLocation = true
		} else {
			params.Location = pgtype.Text{String: v, Valid: true}
		}
	}

	rowsAffected, err := s.queries.TaskUpdate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.ConstraintName {
			case dbconst.ConstraintDcimTasksFkAssignee:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("assignee not found"))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update task: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("task not found"))
	}

	s.logger.InfoContext(ctx, "task updated", "task_id", taskID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
