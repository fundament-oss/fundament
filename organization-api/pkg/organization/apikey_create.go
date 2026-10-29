package organization

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/common/apitoken"
	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func (s *Server) CreateAPIKey(
	ctx context.Context,
	req *organizationv1.CreateAPIKeyRequest,
) (*organizationv1.CreateAPIKeyResponse, error) {
	organizationID, ok := OrganizationIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("organization_id missing from context"))
	}

	if err := s.checkPermission(ctx, authz.CanCreateApikey(), authz.Organization(organizationID)); err != nil {
		return nil, err
	}

	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("user_id missing from context"))
	}

	token, hash, err := apitoken.GenerateToken()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to generate token: %w", err))
	}

	expiresIn, err := parseAPIKeyExpiresIn(req.GetExpiresIn(), s.config.APIKeyMaxExpiresIn)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	expires := pgtype.Timestamptz{Valid: false}
	if expiresIn > 0 {
		expires = pgtype.Timestamptz{
			Time:  s.clock.Now().Add(expiresIn),
			Valid: true,
		}
	}

	prefix := apitoken.GetPrefix(token)

	params := db.APIKeyCreateParams{
		OrganizationID: organizationID,
		UserID:         userID,
		Name:           req.GetName(),
		TokenHash:      hash,
		TokenPrefix:    prefix,
		Expires:        expires,
	}

	id, err := s.queries.APIKeyCreate(ctx, params)
	if err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
			if pgErr.Code == pgerrcode.UniqueViolation && pgErr.ConstraintName == dbconst.ConstraintApiKeysUqName {
				return nil, connect.NewError(connect.CodeAlreadyExists,
					fmt.Errorf("an API key with this name already exists"))
			}
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create api key: %w", err))
	}

	s.logger.InfoContext(ctx, "api key created",
		"api_key_id", id,
		"organization_id", organizationID,
		"user_id", userID,
		"name", req.GetName(),
	)

	return organizationv1.CreateAPIKeyResponse_builder{
		Id:          id.String(),
		Token:       token,
		TokenPrefix: prefix,
	}.Build(), nil
}

// parseAPIKeyExpiresIn returns the requested lifetime, or 0 for a key that never expires.
func parseAPIKeyExpiresIn(value string, maximum time.Duration) (time.Duration, error) {
	if value == "" {
		if maximum > 0 {
			return 0, fmt.Errorf("expires_in: must be set, at most %s", formatAPIKeyExpiresIn(maximum))
		}
		return 0, nil
	}

	expiresIn, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("expires_in: must be a duration such as 300s, 5m or 1h")
	}
	if expiresIn <= 0 {
		return 0, fmt.Errorf("expires_in: must be positive")
	}
	if maximum > 0 && expiresIn > maximum {
		return 0, fmt.Errorf("expires_in: must be at most %s", formatAPIKeyExpiresIn(maximum))
	}
	return expiresIn, nil
}

// formatAPIKeyExpiresIn drops the zero units time.Duration.String adds: 8760h, not 8760h0m0s.
func formatAPIKeyExpiresIn(d time.Duration) string {
	s := d.String()
	if trimmed, ok := strings.CutSuffix(s, "m0s"); ok {
		s = trimmed + "m"
	}
	if trimmed, ok := strings.CutSuffix(s, "h0m"); ok {
		s = trimmed + "h"
	}
	return s
}
