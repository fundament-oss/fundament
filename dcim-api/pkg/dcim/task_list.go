package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) ListTasks(
	ctx context.Context,
	req *dcimv1.ListTasksRequest,
) (*dcimv1.ListTasksResponse, error) {
	params := db.TaskListParams{}

	if req.HasStatus() {
		params.Status = pgtype.Text{String: taskStatusFromProto(req.GetStatus()), Valid: true}
	}

	if req.HasPriority() {
		params.Priority = pgtype.Text{String: taskPriorityFromProto(req.GetPriority()), Valid: true}
	}

	if req.HasCategory() {
		params.Category = pgtype.Text{String: taskCategoryFromProto(req.GetCategory()), Valid: true}
	}

	if req.HasAssigneeId() {
		assigneeID, err := uuid.Parse(req.GetAssigneeId())
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid assignee_id: %w", err))
		}

		params.AssigneeID = pgtype.UUID{Bytes: assigneeID, Valid: true}
	}

	rows, err := s.queries.TaskList(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list tasks: %w", err))
	}

	tasks := make([]*dcimv1.Task, 0, len(rows))
	for _, row := range rows {
		tasks = append(tasks, taskFromListRow(&row))
	}

	return dcimv1.ListTasksResponse_builder{
		Tasks: tasks,
	}.Build(), nil
}
