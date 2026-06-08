package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) ListTasks(
	ctx context.Context,
	req *connect.Request[dcimv1.ListTasksRequest],
) (*connect.Response[dcimv1.ListTasksResponse], error) {
	params := db.TaskListParams{}

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

	rows, err := s.queries.TaskList(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list tasks: %w", err))
	}

	tasks := make([]*dcimv1.Task, 0, len(rows))
	for _, row := range rows {
		tasks = append(tasks, taskFromListRow(&row))
	}

	return connect.NewResponse(dcimv1.ListTasksResponse_builder{
		Tasks: tasks,
	}.Build()), nil
}
