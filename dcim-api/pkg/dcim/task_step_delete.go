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

func (s *Server) DeleteTaskStep(
	ctx context.Context,
	req *connect.Request[dcimv1.DeleteTaskStepRequest],
) (*connect.Response[emptypb.Empty], error) {
	taskStepID := uuid.MustParse(req.Msg.GetId())

	rowsAffected, err := s.queries.TaskStepDelete(ctx, db.TaskStepDeleteParams{ID: taskStepID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete task step: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("task step not found"))
	}

	s.logger.InfoContext(ctx, "task step deleted", "task_step_id", taskStepID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
