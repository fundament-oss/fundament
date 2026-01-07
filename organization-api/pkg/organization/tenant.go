package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/organization-api/pkg/models"
	"github.com/fundament-oss/fundament/organization-api/pkg/organization/adapter"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *OrganizationServer) GetTenant(
	ctx context.Context,
	req *connect.Request[organizationv1.GetTenantRequest],
) (*connect.Response[organizationv1.GetTenantResponse], error) {
	tenantID, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid tenant id: %w", err))
	}

	input := models.TenantGet{ID: tenantID}
	if err := s.validator.Validate(input); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	queries, release, err := s.queryProvider.obtainQueries(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to obtain queries", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to obtain queries: %w", err))
	}
	defer release()

	tenant, err := queries.TenantGetByID(ctx, input.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("tenant not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get tenant: %w", err))
	}

	return connect.NewResponse(&organizationv1.GetTenantResponse{
		Tenant: adapter.FromTenant(tenant),
	}), nil
}

func (s *OrganizationServer) UpdateTenant(
	ctx context.Context,
	req *connect.Request[organizationv1.UpdateTenantRequest],
) (*connect.Response[organizationv1.UpdateTenantResponse], error) {
	tenantID, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid tenant id: %w", err))
	}

	input := models.TenantUpdate{ID: tenantID, Name: req.Msg.Name}
	if err := s.validator.Validate(input); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	queries, release, err := s.queryProvider.obtainQueries(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to obtain queries: %w", err))
	}
	defer release()

	params := db.TenantUpdateParams{
		ID:   input.ID,
		Name: input.Name,
	}

	tenant, err := queries.TenantUpdate(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("tenant not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update tenant: %w", err))
	}

	s.logger.InfoContext(ctx, "tenant updated", "tenant_id", tenant.ID, "name", tenant.Name)

	return connect.NewResponse(&organizationv1.UpdateTenantResponse{
		Tenant: adapter.FromTenant(tenant),
	}), nil
}
