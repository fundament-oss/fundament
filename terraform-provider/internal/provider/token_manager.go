package provider

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"

	authnv1 "github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1"
	"github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1/authnv1connect"
	"github.com/fundament-oss/fundament/common/apitoken"
)

const (
	// RefreshBuffer is how long before expiration to refresh the token.
	RefreshBuffer = 2 * time.Minute
)

// TokenManager handles API key to JWT exchange and automatic refresh.
type TokenManager struct {
	mu            sync.RWMutex
	apiKey        string
	authnEndpoint string

	token     string
	expiresAt time.Time

	client authnv1connect.TokenServiceClient
}

// NewTokenManager creates a new token manager for API key authentication.
func NewTokenManager(apiKey, authnEndpoint string) *TokenManager {
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	return &TokenManager{
		apiKey:        apiKey,
		authnEndpoint: authnEndpoint,
		client:        authnv1connect.NewTokenServiceClient(httpClient, authnEndpoint),
	}
}

// GetToken returns a valid JWT, refreshing if necessary.
func (tm *TokenManager) GetToken(ctx context.Context) (string, error) {
	tm.mu.RLock()
	if tm.token != "" && time.Now().Add(RefreshBuffer).Before(tm.expiresAt) {
		token := tm.token
		tm.mu.RUnlock()
		return token, nil
	}
	tm.mu.RUnlock()

	// Need to refresh
	return tm.refreshToken(ctx)
}

// refreshToken exchanges the API key for a new JWT.
func (tm *TokenManager) refreshToken(ctx context.Context) (string, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Double-check in case another goroutine already refreshed
	if tm.token != "" && time.Now().Add(RefreshBuffer).Before(tm.expiresAt) {
		return tm.token, nil
	}

	// Create request with API key in Authorization header
	req := connect.NewRequest(&authnv1.ExchangeTokenRequest{})
	req.Header().Set("Authorization", "Bearer "+tm.apiKey)

	resp, err := tm.client.ExchangeToken(ctx, req)
	if err != nil {
		return "", fmt.Errorf("token exchange failed for API key %q at %s: %w", apitoken.GetPrefix(tm.apiKey), tm.authnEndpoint, err)
	}

	// Parse expiration from the JWT itself
	parser := jwt.NewParser()
	token, _, err := parser.ParseUnverified(resp.Msg.AccessToken, jwt.MapClaims{})
	if err != nil {
		return "", fmt.Errorf("failed to parse token: %w", err)
	}

	exp, err := token.Claims.GetExpirationTime()
	if err != nil {
		return "", fmt.Errorf("failed to get token expiration: %w", err)
	}

	if exp == nil {
		return "", fmt.Errorf("token missing expiration claim")
	}

	tm.token = token.Raw
	tm.expiresAt = exp.Time

	return tm.token, nil
}

// StaticTokenSource provides a fixed token.
type StaticTokenSource string

// GetToken returns the static token.
func (s StaticTokenSource) GetToken(_ context.Context) (string, error) {
	return string(s), nil
}
