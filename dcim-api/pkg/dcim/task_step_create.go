package dcim

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) CreateTaskStep(
	ctx context.Context,
	req *connect.Request[dcimv1.CreateTaskStepRequest],
) (*connect.Response[dcimv1.CreateTaskStepResponse], error) {
	params := db.TaskStepCreateParams{
		TaskID:  uuid.MustParse(req.Msg.GetTaskId()),
		Title:   req.Msg.GetTitle(),
		Ordinal: req.Msg.GetOrdinal(),
	}

	if req.Msg.HasDescription() {
		params.Description = pgtype.Text{String: req.Msg.GetDescription(), Valid: true}
	}

	id, err := s.queries.TaskStepCreate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == dbconst.ConstraintDcimTaskStepsFkTask {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("task not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create task step: %w", err))
	}

	s.logger.InfoContext(ctx, "task step created", "task_step_id", id)

	return connect.NewResponse(dcimv1.CreateTaskStepResponse_builder{
		TaskStepId: id.String(),
	}.Build()), nil
}
