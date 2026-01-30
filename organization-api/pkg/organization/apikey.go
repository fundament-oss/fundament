package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/fundament-oss/fundament/common/apitoken"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	"github.com/fundament-oss/fundament/organization-api/pkg/organization/adapter"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *OrganizationServer) CreateAPIKey(
	ctx context.Context,
	req *connect.Request[organizationv1.CreateAPIKeyRequest],
) (*connect.Response[organizationv1.CreateAPIKeyResponse], error) {
	input := adapter.ToAPIKeyCreate(req.Msg)
	if err := s.validator.Validate(input); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	organizationID, ok := OrganizationIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	claims, ok := ClaimsFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("claims missing from context"))
	}

	token, hash, err := apitoken.GenerateToken()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to generate token: %w", err))
	}

	prefix := apitoken.GetPrefix(token)

	params := db.APIKeyCreateParams{
		OrganizationID: organizationID,
		UserID:         claims.UserID,
		Name:           input.Name,
		TokenHash:      hash,
		TokenPrefix:    prefix,
		Expires:        adapter.ToExpires(input.ExpiresInDays),
	}

	id, err := s.queries.APIKeyCreate(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create api key: %w", err))
	}

	s.logger.InfoContext(ctx, "api key created",
		"api_key_id", id,
		"organization_id", organizationID,
		"user_id", claims.UserID,
		"name", input.Name,
	)

	return connect.NewResponse(&organizationv1.CreateAPIKeyResponse{
		Id:          id.String(),
		Token:       token,
		TokenPrefix: prefix,
	}), nil
}

func (s *OrganizationServer) ListAPIKeys(
	ctx context.Context,
	req *connect.Request[organizationv1.ListAPIKeysRequest],
) (*connect.Response[organizationv1.ListAPIKeysResponse], error) {
	organizationID, ok := OrganizationIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
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

	return connect.NewResponse(&organizationv1.ListAPIKeysResponse{
		ApiKeys: adapter.FromAPIKeys(keys),
	}), nil
}

func (s *OrganizationServer) GetAPIKey(
	ctx context.Context,
	req *connect.Request[organizationv1.GetAPIKeyRequest],
) (*connect.Response[organizationv1.GetAPIKeyResponse], error) {
	apiKeyID, err := uuid.Parse(req.Msg.ApiKeyId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid api_key_id: %w", err))
	}

	key, err := s.queries.APIKeyGetByID(ctx, db.APIKeyGetByIDParams{
		ID: apiKeyID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("api key not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get api key: %w", err))
	}

	return connect.NewResponse(&organizationv1.GetAPIKeyResponse{
		ApiKey: adapter.FromAPIKey(&key),
	}), nil
}

func (s *OrganizationServer) RevokeAPIKey(
	ctx context.Context,
	req *connect.Request[organizationv1.RevokeAPIKeyRequest],
) (*connect.Response[emptypb.Empty], error) {
	apiKeyID, err := uuid.Parse(req.Msg.ApiKeyId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid api_key_id: %w", err))
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

func (s *OrganizationServer) DeleteAPIKey(
	ctx context.Context,
	req *connect.Request[organizationv1.DeleteAPIKeyRequest],
) (*connect.Response[emptypb.Empty], error) {
	apiKeyID, err := uuid.Parse(req.Msg.ApiKeyId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid api_key_id: %w", err))
	}

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
