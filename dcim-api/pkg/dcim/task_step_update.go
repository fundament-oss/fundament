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

func (s *Server) UpdateTaskStep(
	ctx context.Context,
	req *connect.Request[dcimv1.UpdateTaskStepRequest],
) (*connect.Response[emptypb.Empty], error) {
	taskStepID := uuid.MustParse(req.Msg.GetId())

	params := db.TaskStepUpdateParams{
		ID: taskStepID,
	}

	if req.Msg.HasTitle() {
		params.Title = pgtype.Text{String: req.Msg.GetTitle(), Valid: true}
	}

	if req.Msg.HasDescription() {
		params.Description = pgtype.Text{String: req.Msg.GetDescription(), Valid: true}
	}

	if req.Msg.HasOrdinal() {
		params.Ordinal = pgtype.Int4{Int32: req.Msg.GetOrdinal(), Valid: true}
	}

	if req.Msg.HasCompleted() {
		params.Completed = pgtype.Bool{Bool: req.Msg.GetCompleted(), Valid: true}
	}

	rowsAffected, err := s.queries.TaskStepUpdate(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update task step: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("task step not found"))
	}

	s.logger.InfoContext(ctx, "task step updated", "task_step_id", taskStepID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
