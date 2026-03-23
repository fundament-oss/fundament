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

func (s *Server) RevokeAPIKey(
	ctx context.Context,
	req *connect.Request[organizationv1.RevokeAPIKeyRequest],
) (*connect.Response[organizationv1.RevokeAPIKeyResponse], error) {
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
		if err := s.checkPermission(ctx, authz.CanEditApikey(), authz.Organization(key.OrganizationID)); err != nil {
			return nil, err
		}
	}

	rowsAffected, err := s.queries.APIKeyRevoke(ctx, db.APIKeyRevokeParams{ID: apiKeyID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to revoke api key: %w", err))
	}

	if rowsAffected != 1 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("api key not found or already revoked"))
	}

	s.logger.InfoContext(ctx, "api key revoked", "api_key_id", apiKeyID)

	return connect.NewResponse(organizationv1.RevokeAPIKeyResponse_builder{}.Build()), nil
}
