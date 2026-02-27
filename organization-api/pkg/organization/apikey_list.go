package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) ListAPIKeys(
	ctx context.Context,
	req *connect.Request[organizationv1.ListAPIKeysRequest],
) (*connect.Response[organizationv1.ListAPIKeysResponse], error) {
	organizationID, ok := OrganizationIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	if err := s.checkPermission(ctx, authz.CanListApikeys(), authz.Organization(organizationID)); err != nil {
		return nil, err
	}

	claims, ok := ClaimsFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("claims missing from context"))
	}

	keys, err := s.queries.APIKeyListByOrganizationID(ctx, db.APIKeyListByOrganizationIDParams{
		OrganizationID: organizationID,
		UserID:         claims.UserID,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list api keys: %w", err))
	}

	result := make([]*organizationv1.APIKey, 0, len(keys))
	for idx := range keys {
		result = append(result, apiKeyFromListRow(&keys[idx]))
	}

	return connect.NewResponse(organizationv1.ListAPIKeysResponse_builder{
		ApiKeys: result,
	}.Build()), nil
}
