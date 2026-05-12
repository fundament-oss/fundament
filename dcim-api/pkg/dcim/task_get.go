package dcim

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) GetTask(
	ctx context.Context,
	req *connect.Request[dcimv1.GetTaskRequest],
) (*connect.Response[dcimv1.GetTaskResponse], error) {
	taskID := uuid.MustParse(req.Msg.GetId())

	task, err := s.queries.TaskGetByID(ctx, db.TaskGetByIDParams{ID: taskID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("task not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get task: %w", err))
	}

	return connect.NewResponse(dcimv1.GetTaskResponse_builder{
		Task: taskFromRow(&task),
	}.Build()), nil
}
