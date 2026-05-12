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

func (s *Server) GetDesign(
	ctx context.Context,
	req *connect.Request[dcimv1.GetDesignRequest],
) (*connect.Response[dcimv1.GetDesignResponse], error) {
	designID := uuid.MustParse(req.Msg.GetId())

	design, err := s.queries.LogicalDesignGetByID(ctx, db.LogicalDesignGetByIDParams{ID: designID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("design not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get design: %w", err))
	}

	return connect.NewResponse(dcimv1.GetDesignResponse_builder{
		Design: designFromRow(&design),
	}.Build()), nil
}
