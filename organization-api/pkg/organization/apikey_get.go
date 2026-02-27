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

func (s *Server) GetAPIKey(
	ctx context.Context,
	req *connect.Request[organizationv1.GetAPIKeyRequest],
) (*connect.Response[organizationv1.GetAPIKeyResponse], error) {
	apiKeyID := uuid.MustParse(req.Msg.GetApiKeyId())

	if err := s.checkPermission(ctx, authz.CanView(), authz.ApiKey(apiKeyID)); err != nil {
		return nil, err
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

	return connect.NewResponse(organizationv1.GetAPIKeyResponse_builder{
		ApiKey: apiKeyFromGetRow(&key),
	}.Build()), nil
}
