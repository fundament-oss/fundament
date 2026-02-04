package organization

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) DeleteAPIKey(
	ctx context.Context,
	req *connect.Request[organizationv1.DeleteAPIKeyRequest],
) (*connect.Response[emptypb.Empty], error) {
	apiKeyID := uuid.MustParse(req.Msg.ApiKeyId)

	rowsAffected, err := s.queries.APIKeyDelete(ctx, db.APIKeyDeleteParams{ID: apiKeyID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete api key: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("api key not found"))
	}

	s.logger.InfoContext(ctx, "api key deleted", "api_key_id", apiKeyID)

	return connect.NewResponse(&emptypb.Empty{}), nil
}
