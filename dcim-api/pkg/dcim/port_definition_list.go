package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) ListPortDefinitions(
	ctx context.Context,
	req *connect.Request[dcimv1.ListPortDefinitionsRequest],
) (*connect.Response[dcimv1.ListPortDefinitionsResponse], error) {
	catalogID := uuid.MustParse(req.Msg.GetDeviceCatalogId())

	rows, err := s.queries.PortDefinitionList(ctx, db.PortDefinitionListParams{DeviceCatalogID: catalogID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list port definitions: %w", err))
	}

	portDefs := make([]*dcimv1.PortDefinition, 0, len(rows))
	for _, row := range rows {
		portDefs = append(portDefs, portDefinitionFromListRow(&row))
	}

	return connect.NewResponse(dcimv1.ListPortDefinitionsResponse_builder{
		PortDefinitions: portDefs,
	}.Build()), nil
}
