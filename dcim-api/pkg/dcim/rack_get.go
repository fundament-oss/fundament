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

func (s *Server) GetRack(
	ctx context.Context,
	req *dcimv1.GetRackRequest,
) (*dcimv1.GetRackResponse, error) {
	rackID := uuid.MustParse(req.GetId())

	row, err := s.queries.RackGetByID(ctx, db.RackGetByIDParams{ID: rackID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("rack not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get rack: %w", err))
	}

	return dcimv1.GetRackResponse_builder{
		Rack: rackFromGetRow(&row),
	}.Build(), nil
}
