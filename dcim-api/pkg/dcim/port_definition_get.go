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

func (s *Server) GetPortDefinition(
	ctx context.Context,
	req *connect.Request[dcimv1.GetPortDefinitionRequest],
) (*connect.Response[dcimv1.GetPortDefinitionResponse], error) {
	portDefID := uuid.MustParse(req.Msg.GetId())

	row, err := s.queries.PortDefinitionGetByID(ctx, db.PortDefinitionGetByIDParams{ID: portDefID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("port definition not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get port definition: %w", err))
	}

	return connect.NewResponse(dcimv1.GetPortDefinitionResponse_builder{
		PortDefinition: portDefinitionFromGetRow(&row),
	}.Build()), nil
}
