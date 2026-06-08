package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/emptypb"

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

	if req.Msg.HasDescription() {
		params.Description = pgtype.Text{String: req.Msg.GetDescription(), Valid: true}
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

	if req.Msg.HasAssigneeId() {
		params.AssigneeID = pgtype.Text{String: req.Msg.GetAssigneeId(), Valid: true}
	}

	if req.Msg.HasDueDate() {
		params.DueDate = pgtype.Timestamptz{Time: req.Msg.GetDueDate().AsTime(), Valid: true}
	}

	if req.Msg.HasLocation() {
		params.Location = pgtype.Text{String: req.Msg.GetLocation(), Valid: true}
	}

	rowsAffected, err := s.queries.TaskUpdate(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update task: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("task not found"))
	}

	s.logger.InfoContext(ctx, "task updated", "task_id", taskID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
