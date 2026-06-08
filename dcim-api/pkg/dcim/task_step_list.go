package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) ListTaskSteps(
	ctx context.Context,
	req *connect.Request[dcimv1.ListTaskStepsRequest],
) (*connect.Response[dcimv1.ListTaskStepsResponse], error) {
	taskID := uuid.MustParse(req.Msg.GetTaskId())

	rows, err := s.queries.TaskStepList(ctx, db.TaskStepListParams{TaskID: taskID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list task steps: %w", err))
	}

	steps := make([]*dcimv1.TaskStep, 0, len(rows))
	for _, row := range rows {
		steps = append(steps, taskStepFromListRow(&row))
	}

	return connect.NewResponse(dcimv1.ListTaskStepsResponse_builder{
		Steps: steps,
	}.Build()), nil
}
