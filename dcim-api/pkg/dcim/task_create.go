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

func (s *Server) CreateTask(
	ctx context.Context,
	req *connect.Request[dcimv1.CreateTaskRequest],
) (*connect.Response[dcimv1.CreateTaskResponse], error) {
	params := db.TaskCreateParams{
		Title:    req.Msg.GetTitle(),
		Status:   taskStatusFromProto(req.Msg.GetStatus()),
		Priority: taskPriorityFromProto(req.Msg.GetPriority()),
		Category: taskCategoryFromProto(req.Msg.GetCategory()),
	}

	if req.Msg.HasDescription() {
		params.Description = pgtype.Text{String: req.Msg.GetDescription(), Valid: true}
	}

	if req.Msg.HasAssigneeId() {
		assigneeID, err := uuid.Parse(req.Msg.GetAssigneeId())
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid assignee_id: %w", err))
		}

		params.AssigneeID = pgtype.UUID{Bytes: assigneeID, Valid: true}
	}

	if req.Msg.HasDueDate() {
		params.DueDate = pgtype.Timestamptz{Time: req.Msg.GetDueDate().AsTime(), Valid: true}
	}

	if req.Msg.HasLocation() {
		params.Location = pgtype.Text{String: req.Msg.GetLocation(), Valid: true}
	}

	id, err := s.queries.TaskCreate(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.ConstraintName {
			case dbconst.ConstraintDcimTasksFkAssignee:
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("assignee not found"))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create task: %w", err))
	}

	s.logger.InfoContext(ctx, "task created", "task_id", id)

	return connect.NewResponse(dcimv1.CreateTaskResponse_builder{
		TaskId: id.String(),
	}.Build()), nil
}
