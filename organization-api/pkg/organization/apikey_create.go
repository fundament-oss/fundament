package organization

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/common/apitoken"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) CreateAPIKey(
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

func toExpires(expiresInDays *int64) pgtype.Timestamptz {
	if expiresInDays == nil {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{
		Time:  time.Now().AddDate(0, 0, int(*expiresInDays)),
		Valid: true,
	}
}
