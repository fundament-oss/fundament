package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/fundament-oss/fundament/common/authz"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) DeleteAPIKey(
	ctx context.Context,
	req *connect.Request[organizationv1.DeleteAPIKeyRequest],
) (*connect.Response[organizationv1.DeleteAPIKeyResponse], error) {
	apiKeyID := uuid.MustParse(req.Msg.GetApiKeyId())

	key, err := s.queries.APIKeyGetByID(ctx, db.APIKeyGetByIDParams{ID: apiKeyID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("api key not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get api key: %w", err))
	}

	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("user_id missing from context"))
	}

	if key.UserID != userID {
		if err := s.checkPermission(ctx, authz.CanDeleteApikey(), authz.Organization(key.OrganizationID)); err != nil {
			return nil, err
		}
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
