package authn

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	db "github.com/fundament-oss/fundament/authn-api/pkg/db/gen"
	authnv1 "github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1"
	"github.com/fundament-oss/fundament/common/apitoken"
	"github.com/fundament-oss/fundament/common/authz"
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
	token, err := extractBearerToken(req.Header().Get("Authorization"))
	if err != nil {
		s.logger.Debug("missing or invalid authorization header")
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	s.logger.Debug("token exchange attempt", "token_length", len(token), "token_prefix", apitoken.GetPrefix(token))

	if !apitoken.IsAPIToken(token) {
		s.logger.Debug("token does not look like API token", "starts_with", token[:min(8, len(token))])
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid token format"))
	}

	if err := apitoken.ValidateFormat(token); err != nil {
		s.logger.Debug("invalid api token format", "error", err, "token_length", len(token))
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid token: %w", err))
	}

	apiKey, err := s.lookupAPIKey(ctx, token)
	if err != nil {
		return nil, err
	}

	dbUser, err := s.queries.UserGetByID(ctx, db.UserGetByIDParams{ID: apiKey.UserID})
	if err != nil {
		s.logger.Error("failed to get user for api key", "error", err, "api_key_id", apiKey.ID, "user_id", apiKey.UserID)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("internal error"))
	}

	organizationIDs, err := s.getUserOrganizationIDs(ctx, dbUser.ID)
	if err != nil {
		s.logger.Error("failed to get user organizations for api key", "error", err, "api_key_id", apiKey.ID, "user_id", apiKey.UserID)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("internal error"))
	}

	u := &user{
		ID:              dbUser.ID,
		OrganizationIDs: organizationIDs,
		Name:            dbUser.Name,
		ExternalRef:     dbUser.ExternalRef.String,
	}

	accessToken, err := s.generateJWTWithExpiry(u, []string{}, APITokenExpiry)
	if err != nil {
		s.logger.Error("failed to generate jwt for api token", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("internal error"))
	}

	s.logger.Info("api token exchanged for jwt",
		"api_key_id", apiKey.ID,
		"user_id", dbUser.ID,
		"organization_ids", u.OrganizationIDs,
	)

	return connect.NewResponse(authnv1.ExchangeTokenResponse_builder{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int64(APITokenExpiry.Seconds()),
	}.Build()), nil
}

// extractBearerToken extracts the token from a Bearer authorization header.
func extractBearerToken(authHeader string) (string, error) {
	if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
		return "", fmt.Errorf("missing or invalid authorization header")
	}
	return authHeader[7:], nil
}

// lookupAPIKey looks up an API key by token hash and validates it via OpenFGA.
func (s *AuthnServer) lookupAPIKey(ctx context.Context, token string) (*db.APIKeyGetByHashRow, error) {
	hash := apitoken.Hash(token)
	apiKey, err := s.queries.APIKeyGetByHash(ctx, db.APIKeyGetByHashParams{
		PTokenHash: hash,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.logger.Debug("api token not found")
			return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid token"))
		}
		s.logger.Error("failed to get api key", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("internal error"))
	}

	decision, err := s.authz.Evaluate(ctx, authz.EvaluationRequest{
		Subject:  authz.User(apiKey.UserID),
		Action:   authz.CanUse(),
		Resource: authz.ApiKey(apiKey.ID),
		Context:  authz.Context{"current_time": time.Now().UTC().Format(time.RFC3339)},
	})
	if err != nil {
		s.logger.Error("failed to evaluate api key authorization", "error", err, "api_key_id", apiKey.ID)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("internal error"))
	}

	if !decision.Decision {
		s.logger.Debug("api key authorization denied", "api_key_id", apiKey.ID)
		return nil, connect.NewError(connect.CodeUnauthenticated, s.apiKeyDenialReason(apiKey))
	}

	if err := s.queries.APIKeyUpdateLastUsed(ctx, db.APIKeyUpdateLastUsedParams{PID: apiKey.ID}); err != nil {
		s.logger.Error("failed to update api key last_used", "error", err, "api_key_id", apiKey.ID)
	}

	return &apiKey, nil
}

// apiKeyDenialReason returns a user-facing error based on the API key's state.
func (s *AuthnServer) apiKeyDenialReason(apiKey db.APIKeyGetByHashRow) error {
	if apiKey.Revoked.Valid {
		return fmt.Errorf("token revoked")
	}
	if apiKey.Expires.Valid && apiKey.Expires.Time.Before(time.Now()) {
		return fmt.Errorf("token expired")
	}
	return fmt.Errorf("invalid token")
}
