package authn

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	db "github.com/fundament-oss/fundament/authn-api/pkg/db/gen"
	authnv1 "github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1"
	"github.com/fundament-oss/fundament/common/apitoken"
	"github.com/fundament-oss/fundament/common/dbconst"
)

const (
	// APITokenExpiry is the expiry time for JWTs issued from API token exchange.
	// Shorter than user session tokens for security.
	APITokenExpiry = 15 * time.Minute
)

// ExchangeToken exchanges an API token for a short-lived JWT.
func (s *AuthnServer) ExchangeToken(
	ctx context.Context,
	req *connect.Request[authnv1.ExchangeTokenRequest],
) (*connect.Response[authnv1.ExchangeTokenResponse], error) {
	// Extract API token from Authorization header
	authHeader := req.Header().Get("Authorization")
	if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
		s.logger.Debug("missing or invalid authorization header", "header_length", len(authHeader))
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("missing or invalid authorization header"))
	}
	token := authHeader[7:]

	s.logger.Debug("token exchange attempt", "token_length", len(token), "token_prefix", apitoken.GetPrefix(token))

	// Check if this looks like an API token
	if !apitoken.IsAPIToken(token) {
		s.logger.Debug("token does not look like API token", "starts_with", token[:min(8, len(token))])
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid token format"))
	}

	// Validate token format (prefix + CRC32 checksum) without DB hit
	if err := apitoken.ValidateFormat(token); err != nil {
		s.logger.Debug("invalid api token format", "error", err, "token_length", len(token))
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid token: %w", err))
	}

	// Look up token by hash (also checks deleted, revoked, and expired in the DB)
	hash := apitoken.Hash(token)
	apiKey, err := s.queries.APIKeyGetByHash(ctx, db.APIKeyGetByHashParams{
		PTokenHash: hash,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.logger.Debug("api token not found")
			return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid token"))
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Hint {
			case dbconst.HintApiKeyDeleted:
				s.logger.Debug("api token deleted")
				return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid token"))
			case dbconst.HintApiKeyRevoked:
				s.logger.Debug("api token revoked")
				return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("token revoked"))
			case dbconst.HintApiKeyExpired:
				s.logger.Debug("api token expired")
				return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("token expired"))
			}
		}

		s.logger.Error("failed to get api key", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("internal error"))
	}

	// Get the user associated with this API key
	dbUser, err := s.queries.UserGetByID(ctx, db.UserGetByIDParams{ID: apiKey.UserID})
	if err != nil {
		s.logger.Error("failed to get user for api key", "error", err, "api_key_id", apiKey.ID, "user_id", apiKey.UserID)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("internal error"))
	}

	// Generate a short-lived JWT (empty groups - authorization uses DB role, not JWT)
	accessToken, err := s.generateJWTWithExpiry(&user{
		ID:             dbUser.ID,
		OrganizationID: dbUser.OrganizationID,
		Name:           dbUser.Name,
		ExternalID:     dbUser.ExternalID.String,
	}, []string{}, APITokenExpiry)
	if err != nil {
		s.logger.Error("failed to generate jwt for api token", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("internal error"))
	}

	s.logger.Info("api token exchanged for jwt",
		"api_key_id", apiKey.ID,
		"user_id", dbUser.ID,
		"organization_id", dbUser.OrganizationID,
	)

	return connect.NewResponse(&authnv1.ExchangeTokenResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int64(APITokenExpiry.Seconds()),
	}), nil
}
