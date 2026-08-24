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
	req *dcimv1.UpdateTaskStepRequest,
) (*emptypb.Empty, error) {
	taskStepID := uuid.MustParse(req.GetId())

	params := db.TaskStepUpdateParams{
		ID: taskStepID,
	}

	if req.HasTitle() {
		params.Title = pgtype.Text{String: req.GetTitle(), Valid: true}
	}

	if req.HasDescription() {
		params.Description = pgtype.Text{String: req.GetDescription(), Valid: true}
	}

	if req.HasOrdinal() {
		params.Ordinal = pgtype.Int4{Int32: req.GetOrdinal(), Valid: true}
	}

	if req.HasCompleted() {
		params.Completed = pgtype.Bool{Bool: req.GetCompleted(), Valid: true}
	}

	rowsAffected, err := s.queries.TaskStepUpdate(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update task step: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("task step not found"))
	}

	s.logger.InfoContext(ctx, "task step updated", "task_step_id", taskStepID)

	return &emptypb.Empty{}, nil
}
