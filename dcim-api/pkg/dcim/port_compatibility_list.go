package dcim

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	db "github.com/fundament-oss/fundament/dcim-api/pkg/db/gen"
	dcimv1 "github.com/fundament-oss/fundament/dcim-api/pkg/proto/gen/v1"
)

func (s *Server) ListPortCompatibilities(
	ctx context.Context,
	req *dcimv1.ListPortCompatibilitiesRequest,
) (*dcimv1.ListPortCompatibilitiesResponse, error) {
	portDefID := uuid.MustParse(req.GetPortDefinitionId())

	rows, err := s.queries.PortCompatibilityList(ctx, db.PortCompatibilityListParams{PortDefinitionID: portDefID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list port compatibilities: %w", err))
	}

	compatibilities := make([]*dcimv1.PortCompatibility, 0, len(rows))
	for _, row := range rows {
		pc := dcimv1.PortCompatibility_builder{
			PortDefinitionId:   row.PortDefinitionID.String(),
			CompatibleCategory: assetCategoryToProto(row.CompatibleCategory),
		}.Build()

		if row.CompatibleCatalogID.Valid {
			pc.SetCompatibleCatalogId(uuid.UUID(row.CompatibleCatalogID.Bytes).String())
		}

		compatibilities = append(compatibilities, pc)
	}

	return dcimv1.ListPortCompatibilitiesResponse_builder{
		Compatibilities: compatibilities,
	}.Build(), nil
}
