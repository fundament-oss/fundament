package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) ListDesigns(
	ctx context.Context,
	req *connect.Request[dcimv1.ListDesignsRequest],
) (*connect.Response[dcimv1.ListDesignsResponse], error) {
	rows, err := s.queries.LogicalDesignList(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list designs: %w", err))
	}

	designs := make([]*dcimv1.LogicalDesign, 0, len(rows))
	for _, row := range rows {
		designs = append(designs, designFromListRow(&row))
	}

	return connect.NewResponse(dcimv1.ListDesignsResponse_builder{
		Designs: designs,
	}.Build()), nil
}
