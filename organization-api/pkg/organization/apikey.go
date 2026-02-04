package organization

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fundament-oss/fundament/common/apitoken"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *OrganizationServer) CreateAPIKey(
	ctx context.Context,
	req *connect.Request[organizationv1.CreateAPIKeyRequest],
) (*connect.Response[organizationv1.CreateAPIKeyResponse], error) {
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
		Name:           req.Msg.Name,
		TokenHash:      hash,
		TokenPrefix:    prefix,
		Expires:        toExpires(req.Msg.ExpiresInDays),
	}

	id, err := s.queries.APIKeyCreate(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create api key: %w", err))
	}

	s.logger.InfoContext(ctx, "api key created",
		"api_key_id", id,
		"organization_id", organizationID,
		"user_id", claims.UserID,
		"name", req.Msg.Name,
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

	result := make([]*organizationv1.APIKey, 0, len(keys))
	for idx := range keys {
		result = append(result, apiKeyFromDB((*db.APIKeyGetByIDRow)(&keys[idx])))
	}

	return connect.NewResponse(&organizationv1.ListAPIKeysResponse{
		ApiKeys: result,
	}), nil
}

func (s *OrganizationServer) GetAPIKey(
	ctx context.Context,
	req *connect.Request[organizationv1.GetAPIKeyRequest],
) (*connect.Response[organizationv1.GetAPIKeyResponse], error) {
	apiKeyID := uuid.MustParse(req.Msg.ApiKeyId)

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
		ApiKey: apiKeyFromDB(&key),
	}), nil
}

func (s *OrganizationServer) RevokeAPIKey(
	ctx context.Context,
	req *connect.Request[organizationv1.RevokeAPIKeyRequest],
) (*connect.Response[emptypb.Empty], error) {
	apiKeyID := uuid.MustParse(req.Msg.ApiKeyId)

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

func toExpires(expiresInDays *int64) pgtype.Timestamptz {
	if expiresInDays == nil {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{
		Time:  time.Now().AddDate(0, 0, int(*expiresInDays)),
		Valid: true,
	}
}

func apiKeyFromDB(record *db.APIKeyGetByIDRow) *organizationv1.APIKey {
	apiKey := &organizationv1.APIKey{
		Id:          record.ID.String(),
		Name:        record.Name,
		TokenPrefix: record.TokenPrefix,
		CreatedAt:   timestamppb.New(record.Created.Time),
	}
	if record.Expires.Valid {
		apiKey.ExpiresAt = timestamppb.New(record.Expires.Time)
	}
	if record.LastUsed.Valid {
		apiKey.LastUsedAt = timestamppb.New(record.LastUsed.Time)
	}
	if record.Revoked.Valid {
		apiKey.RevokedAt = timestamppb.New(record.Revoked.Time)
	}
	return apiKey
}
