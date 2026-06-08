package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) DeleteTask(
	ctx context.Context,
	req *connect.Request[dcimv1.DeleteTaskRequest],
) (*connect.Response[emptypb.Empty], error) {
	taskID := uuid.MustParse(req.Msg.GetId())

	rowsAffected, err := s.queries.TaskDelete(ctx, db.TaskDeleteParams{ID: taskID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete task: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("task not found"))
	}

	s.logger.InfoContext(ctx, "task deleted", "task_id", taskID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
