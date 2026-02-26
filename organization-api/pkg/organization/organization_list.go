package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) ListOrganizations(
	ctx context.Context,
	req *connect.Request[organizationv1.ListOrganizationsRequest],
) (*connect.Response[organizationv1.ListOrganizationsResponse], error) {
	orgs, err := s.queries.OrganizationList(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list organizations: %w", err))
	}

	result := make([]*organizationv1.Organization, 0, len(orgs))
	for _, org := range orgs {
		result = append(result, toOrg(org))
	}

	return connect.NewResponse(organizationv1.ListOrganizationsResponse_builder{
		Organizations: result,
	}.Build()), nil
}

func toOrg(org db.OrganizationListRow) *organizationv1.Organization {
	return organizationv1.Organization_builder{
		Id:          org.ID.String(),
		Name:        org.Name,
		DisplayName: org.DisplayName,
		Created:     timestamppb.New(org.Created.Time),
	}.Build()
}
