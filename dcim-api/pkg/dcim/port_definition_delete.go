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

func (s *Server) DeletePortDefinition(
	ctx context.Context,
	req *dcimv1.DeletePortDefinitionRequest,
) (*emptypb.Empty, error) {
	portDefID := uuid.MustParse(req.GetId())

	rowsAffected, err := s.queries.PortDefinitionDelete(ctx, db.PortDefinitionDeleteParams{ID: portDefID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete port definition: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("port definition not found"))
	}

	s.logger.InfoContext(ctx, "port definition deleted", "port_definition_id", portDefID)

	return &emptypb.Empty{}, nil
}
