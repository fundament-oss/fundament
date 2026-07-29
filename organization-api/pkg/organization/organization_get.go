package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) GetOrganization(
	ctx context.Context,
	req *organizationv1.GetOrganizationRequest,
) (*organizationv1.GetOrganizationResponse, error) {
	organizationID := uuid.MustParse(req.GetId())

	if err := s.checkPermission(ctx, authz.CanView(), authz.Organization(organizationID)); err != nil {
		return nil, err
	}

	organization, err := s.queries.OrganizationGetByID(ctx, db.OrganizationGetByIDParams{ID: organizationID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("organization not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get organization: %w", err))
	}

	return organizationv1.GetOrganizationResponse_builder{
		Organization: organizationFromRow(&organization),
	}.Build(), nil
}

func organizationFromRow(row *db.OrganizationGetByIDRow) *organizationv1.Organization {
	return organizationv1.Organization_builder{
		Id:      row.ID.String(),
		Name:    row.Name,
		Alias:   row.Alias,
		Created: timestamppb.New(row.Created.Time),
	}.Build()
}
