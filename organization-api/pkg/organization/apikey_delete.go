package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) DeleteAPIKey(
	ctx context.Context,
	req *connect.Request[organizationv1.DeleteAPIKeyRequest],
) (*connect.Response[organizationv1.DeleteAPIKeyResponse], error) {
	apiKeyID := uuid.MustParse(req.Msg.GetApiKeyId())

	if err := s.checkPermission(ctx, authz.CanDelete(), authz.ApiKey(apiKeyID)); err != nil {
		return nil, err
	}

	rowsAffected, err := s.queries.APIKeyDelete(ctx, db.APIKeyDeleteParams{ID: apiKeyID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete api key: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("api key not found"))
	}

	s.logger.InfoContext(ctx, "api key deleted", "api_key_id", apiKeyID)

	return connect.NewResponse(organizationv1.DeleteAPIKeyResponse_builder{}.Build()), nil
}
