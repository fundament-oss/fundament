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

func (s *Server) RevokeAPIKey(
	ctx context.Context,
	req *connect.Request[organizationv1.RevokeAPIKeyRequest],
) (*connect.Response[organizationv1.RevokeAPIKeyResponse], error) {
	apiKeyID := uuid.MustParse(req.Msg.GetApiKeyId())

	if err := s.checkPermission(ctx, authz.CanEdit(), authz.ApiKey(apiKeyID)); err != nil {
		return nil, err
	}

	claims, ok := ClaimsFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("claims missing from context"))
	}

	rowsAffected, err := s.queries.APIKeyRevoke(ctx, db.APIKeyRevokeParams{ID: apiKeyID, UserID: claims.UserID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to revoke api key: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("api key not found or already revoked"))
	}

	s.logger.InfoContext(ctx, "api key revoked", "api_key_id", apiKeyID)

	return connect.NewResponse(organizationv1.RevokeAPIKeyResponse_builder{}.Build()), nil
}
