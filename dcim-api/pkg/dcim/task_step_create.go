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
	req *dcimv1.CreateTaskStepRequest,
) (*dcimv1.CreateTaskStepResponse, error) {
	params := db.TaskStepCreateParams{
		TaskID:  uuid.MustParse(req.GetTaskId()),
		Title:   req.GetTitle(),
		Ordinal: req.GetOrdinal(),
	}

	if req.HasDescription() {
		params.Description = pgtype.Text{String: req.GetDescription(), Valid: true}
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

	return dcimv1.CreateTaskStepResponse_builder{
		TaskStepId: id.String(),
	}.Build(), nil
}
