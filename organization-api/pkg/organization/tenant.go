package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *OrganizationServer) GetOrganization(
	ctx context.Context,
	req *connect.Request[organizationv1.GetOrganizationRequest],
) (*connect.Response[organizationv1.GetOrganizationResponse], error) {
	organizationID := uuid.MustParse(req.Msg.Id)

	if err := s.checkPermission(ctx, authz.RelationMember, authz.OrganizationObject(organizationID)); err != nil {
		return nil, err
	}

	organization, err := s.queries.OrganizationGetByID(ctx, db.OrganizationGetByIDParams{ID: organizationID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("organization not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get organization: %w", err))
	}

	return connect.NewResponse(&organizationv1.GetOrganizationResponse{
		Organization: &organizationv1.Organization{
			Id:      organization.ID.String(),
			Name:    organization.Name,
			Created: timestamppb.New(organization.Created.Time),
		},
	}), nil
}

func (s *OrganizationServer) UpdateOrganization(
	ctx context.Context,
	req *connect.Request[organizationv1.UpdateOrganizationRequest],
) (*connect.Response[emptypb.Empty], error) {
	organizationID := uuid.MustParse(req.Msg.Id)

	if err := s.checkPermission(ctx, authz.RelationAdmin, authz.OrganizationObject(organizationID)); err != nil {
		return nil, err
	}

	params := db.OrganizationUpdateParams{
		ID:   organizationID,
		Name: req.Msg.Name,
	}

	organization, err := s.queries.OrganizationUpdate(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("organization not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update organization: %w", err))
	}

	s.logger.InfoContext(ctx, "organization updated", "organization_id", organization.ID, "name", organization.Name)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
