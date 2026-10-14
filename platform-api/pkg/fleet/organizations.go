package fleet

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/platform-api/pkg/db/gen"
	fleetv1 "github.com/fundament-oss/fundament/platform-api/pkg/proto/gen/fleet/v1"
)

func (s *Server) ListOrganizations(ctx context.Context, _ *fleetv1.ListOrganizationsRequest) (*fleetv1.ListOrganizationsResponse, error) {
	rows, err := s.queries.OrganizationList(ctx, db.OrganizationListParams{})
	if err != nil {
		return nil, fmt.Errorf("list organizations: %w", err)
	}

	organizations := make([]*fleetv1.OrganizationSummary, 0, len(rows))
	for i := range rows {
		organizations = append(organizations, organizationSummaryFrom(&rows[i]))
	}

	return fleetv1.ListOrganizationsResponse_builder{Organizations: organizations}.Build(), nil
}

func (s *Server) GetOrganization(ctx context.Context, req *fleetv1.GetOrganizationRequest) (*fleetv1.GetOrganizationResponse, error) {
	id := uuid.MustParse(req.GetOrganizationId())

	rows, err := s.queries.OrganizationList(ctx, db.OrganizationListParams{
		OrganizationID: pgtype.UUID{Bytes: id, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("get organization: %w", err)
	}
	if len(rows) == 0 {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("organization not found"))
	}

	return fleetv1.GetOrganizationResponse_builder{Organization: organizationSummaryFrom(&rows[0])}.Build(), nil
}
