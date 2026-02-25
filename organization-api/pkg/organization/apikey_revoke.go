package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) RevokeAPIKey(
	ctx context.Context,
	req *connect.Request[organizationv1.RevokeAPIKeyRequest],
) (*connect.Response[emptypb.Empty], error) {
	apiKeyID := uuid.MustParse(req.Msg.ApiKeyId)

	if err := s.checkPermission(ctx, authz.CanEdit(), authz.ApiKey(apiKeyID)); err != nil {
		return nil, err
	}

	rowsAffected, err := s.queries.APIKeyRevoke(ctx, db.APIKeyRevokeParams{ID: apiKeyID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to revoke api key: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("api key not found or already revoked"))
	}

	s.logger.InfoContext(ctx, "api key revoked", "api_key_id", apiKeyID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
