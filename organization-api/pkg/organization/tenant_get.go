package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) GetOrganization(
	ctx context.Context,
	req *connect.Request[organizationv1.GetOrganizationRequest],
) (*connect.Response[organizationv1.GetOrganizationResponse], error) {
	organizationID := uuid.MustParse(req.Msg.Id)

	organization, err := s.queries.OrganizationGetByID(ctx, db.OrganizationGetByIDParams{ID: organizationID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("organization not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get organization: %w", err))
	}

	return connect.NewResponse(&organizationv1.GetOrganizationResponse{
		Organization: organizationFromRow(&organization),
	}), nil
}

func organizationFromRow(row *db.OrganizationGetByIDRow) *organizationv1.Organization {
	return &organizationv1.Organization{
		Id:      row.ID.String(),
		Name:    row.Name,
		Created: timestamppb.New(row.Created.Time),
	}
}
