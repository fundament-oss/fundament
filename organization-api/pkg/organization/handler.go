package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/organization/v1"
)

func (s *OrganizationServer) GetTenant(
	ctx context.Context,
	req *connect.Request[organizationv1.GetTenantRequest],
) (*connect.Response[organizationv1.GetTenantResponse], error) {
	claims, err := s.validateRequest(req.Header())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid tenant ID: %w", err))
	}

	if claims.TenantID != id {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("access denied to tenant"))
	}

	tenant, err := s.queries.TenantGetByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("tenant not found"))
		}
		s.logger.Error("failed to get tenant", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get tenant: %w", err))
	}

	return connect.NewResponse(&organizationv1.GetTenantResponse{
		Tenant: dbTenantToProto(tenant),
	}), nil
}

func (s *OrganizationServer) UpdateTenant(
	ctx context.Context,
	req *connect.Request[organizationv1.UpdateTenantRequest],
) (*connect.Response[organizationv1.UpdateTenantResponse], error) {
	claims, err := s.validateRequest(req.Header())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid tenant ID: %w", err))
	}

	if claims.TenantID != id {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("access denied to tenant"))
	}

	name := req.Msg.Name
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("tenant name is required"))
	}

	tenant, err := s.queries.TenantUpdate(ctx, db.TenantUpdateParams{
		ID:   id,
		Name: name,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("tenant not found"))
		}
		s.logger.Error("failed to update tenant", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update tenant: %w", err))
	}

	s.logger.Info("tenant updated", "tenant_id", tenant.ID, "name", tenant.Name)

	return connect.NewResponse(&organizationv1.UpdateTenantResponse{
		Tenant: dbTenantToProto(tenant),
	}), nil
}

func dbTenantToProto(t db.OrganizationTenant) *organizationv1.Tenant {
	return &organizationv1.Tenant{
		Id:      t.ID.String(),
		Name:    t.Name,
		Created: t.Created.Time.Format("2006-01-02T15:04:05Z07:00"),
	}
}
