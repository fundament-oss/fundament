package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) UpdateOrganization(
	ctx context.Context,
	req *connect.Request[organizationv1.UpdateOrganizationRequest],
) (*connect.Response[emptypb.Empty], error) {
	organizationID := uuid.MustParse(req.Msg.GetId())

	if err := s.checkPermission(ctx, authz.CanEdit(), authz.Organization(organizationID)); err != nil {
		return nil, err
	}

	params := db.OrganizationUpdateParams{
		ID:          organizationID,
		DisplayName: req.Msg.GetDisplayName(),
	}

	organization, err := s.queries.OrganizationUpdate(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("organization not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update organization: %w", err))
	}

	s.logger.InfoContext(ctx, "organization updated", "organization_id", organization.ID, "name", organization.Name, "display_name", organization.DisplayName)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
