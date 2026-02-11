package provider

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"

	authnv1 "github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1"
	"github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1/authnv1connect"
)

// mockTokenServiceClient is a mock implementation of TokenServiceClient for testing.
type mockTokenServiceClient struct {
	authnv1connect.UnimplementedTokenServiceHandler
	exchangeFunc func(ctx context.Context, req *connect.Request[authnv1.ExchangeTokenRequest]) (*connect.Response[authnv1.ExchangeTokenResponse], error)
}

func (m *mockTokenServiceClient) ExchangeToken(ctx context.Context, req *connect.Request[authnv1.ExchangeTokenRequest]) (*connect.Response[authnv1.ExchangeTokenResponse], error) {
	if m.exchangeFunc != nil {
		return m.exchangeFunc(ctx, req)
	}
	return nil, errors.New("not implemented")
}

// createTestJWT creates a JWT token for testing with the given expiration time.
func createTestJWT(exp time.Time) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp": exp.Unix(),
		"sub": "test-user",
	})
	// Sign with a test secret (signature doesn't matter for ParseUnverified)
	tokenString, _ := token.SignedString([]byte("test-secret"))
	return tokenString
}

func TestStaticTokenSource_GetToken(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		expected string
	}{
		{
			name:     "returns token",
			token:    "my-test-token",
			expected: "my-test-token",
		},
		{
			name:     "returns empty token",
			token:    "",
			expected: "",
		},
		{
			name:     "returns complex token",
			token:    "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
			expected: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := StaticTokenSource(tt.token)
			result, err := source.GetToken(context.Background())
			if err != nil {
				t.Errorf("GetToken() error = %v, want nil", err)
			}
			if result != tt.expected {
				t.Errorf("GetToken() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestTokenManager_GetToken_Fresh(t *testing.T) {
	expTime := time.Now().Add(1 * time.Hour)
	testToken := createTestJWT(expTime)

	mock := &mockTokenServiceClient{
		exchangeFunc: func(ctx context.Context, req *connect.Request[authnv1.ExchangeTokenRequest]) (*connect.Response[authnv1.ExchangeTokenResponse], error) {
			// Verify Authorization header is set
			if req.Header().Get("Authorization") != "Bearer test-api-key" {
				t.Errorf("Authorization header = %q, want %q", req.Header().Get("Authorization"), "Bearer test-api-key")
			}
			return connect.NewResponse(&authnv1.ExchangeTokenResponse{
				AccessToken: testToken,
			}), nil
		},
	}

	tm := &TokenManager{
		apiKey:        "test-api-key",
		authnEndpoint: "http://test.example.com",
		client:        mock,
	}

	token, err := tm.GetToken(context.Background())
	if err != nil {
		t.Fatalf("GetToken() error = %v", err)
	}

	if token != testToken {
		t.Errorf("GetToken() = %q, want %q", token, testToken)
	}
}

func TestTokenManager_GetToken_Cached(t *testing.T) {
	expTime := time.Now().Add(1 * time.Hour)
	testToken := createTestJWT(expTime)

	mock := &mockTokenServiceClient{
		exchangeFunc: func(ctx context.Context, req *connect.Request[authnv1.ExchangeTokenRequest]) (*connect.Response[authnv1.ExchangeTokenResponse], error) {
			return connect.NewResponse(&authnv1.ExchangeTokenResponse{
				AccessToken: testToken,
			}), nil
		},
	}

	tm := &TokenManager{
		apiKey:        "test-api-key",
		authnEndpoint: "http://test.example.com",
		client:        mock,
		token:         testToken,
		expiresAt:     expTime,
	}

	// Get token multiple times
	for range 5 {
		token, err := tm.GetToken(context.Background())
		if err != nil {
			t.Fatalf("GetToken() error = %v", err)
		}
		if token != testToken {
			t.Errorf("GetToken() = %q, want %q", token, testToken)
		}
	}
}

func TestTokenManager_GetToken_RefreshesExpiredToken(t *testing.T) {
	// First token expires soon (within RefreshBuffer)
	oldExpTime := time.Now().Add(1 * time.Minute) // Less than RefreshBuffer
	oldToken := createTestJWT(oldExpTime)

	// New token with longer expiration
	newExpTime := time.Now().Add(1 * time.Hour)
	newToken := createTestJWT(newExpTime)

	mock := &mockTokenServiceClient{
		exchangeFunc: func(ctx context.Context, req *connect.Request[authnv1.ExchangeTokenRequest]) (*connect.Response[authnv1.ExchangeTokenResponse], error) {
			return connect.NewResponse(&authnv1.ExchangeTokenResponse{
				AccessToken: newToken,
			}), nil
		},
	}

	tm := &TokenManager{
		apiKey:        "test-api-key",
		authnEndpoint: "http://test.example.com",
		client:        mock,
		token:         oldToken,
		expiresAt:     oldExpTime, // Expires within RefreshBuffer
	}

	token, err := tm.GetToken(context.Background())
	if err != nil {
		t.Fatalf("GetToken() error = %v", err)
	}

	if token != newToken {
		t.Errorf("GetToken() = %q, want new token", token)
	}
}

func TestTokenManager_GetToken_ExchangeError(t *testing.T) {
	mock := &mockTokenServiceClient{
		exchangeFunc: func(ctx context.Context, req *connect.Request[authnv1.ExchangeTokenRequest]) (*connect.Response[authnv1.ExchangeTokenResponse], error) {
			return nil, errors.New("authentication failed")
		},
	}

	tm := &TokenManager{
		apiKey:        "invalid-api-key",
		authnEndpoint: "http://test.example.com",
		client:        mock,
	}

	_, err := tm.GetToken(context.Background())
	if err == nil {
		t.Fatal("GetToken() expected error, got nil")
	}

	if !errors.Is(err, errors.Unwrap(err)) {
		// Check error message contains relevant info
		if err.Error() != "token exchange failed: authentication failed" {
			t.Errorf("GetToken() error = %q, want to contain 'token exchange failed'", err.Error())
		}
	}
}

func TestTokenManager_GetToken_InvalidJWT(t *testing.T) {
	mock := &mockTokenServiceClient{
		exchangeFunc: func(ctx context.Context, req *connect.Request[authnv1.ExchangeTokenRequest]) (*connect.Response[authnv1.ExchangeTokenResponse], error) {
			return connect.NewResponse(&authnv1.ExchangeTokenResponse{
				AccessToken: "not-a-valid-jwt",
			}), nil
		},
	}

	tm := &TokenManager{
		apiKey:        "test-api-key",
		authnEndpoint: "http://test.example.com",
		client:        mock,
	}

	_, err := tm.GetToken(context.Background())
	if err == nil {
		t.Fatal("GetToken() expected error for invalid JWT, got nil")
	}
}

func TestTokenManager_GetToken_MissingExpiration(t *testing.T) {
	// Create a JWT without exp claim
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "test-user",
	})
	tokenString, _ := token.SignedString([]byte("test-secret"))

	mock := &mockTokenServiceClient{
		exchangeFunc: func(ctx context.Context, req *connect.Request[authnv1.ExchangeTokenRequest]) (*connect.Response[authnv1.ExchangeTokenResponse], error) {
			return connect.NewResponse(&authnv1.ExchangeTokenResponse{
				AccessToken: tokenString,
			}), nil
		},
	}

	tm := &TokenManager{
		apiKey:        "test-api-key",
		authnEndpoint: "http://test.example.com",
		client:        mock,
	}

	_, err := tm.GetToken(context.Background())
	if err == nil {
		t.Fatal("GetToken() expected error for missing expiration, got nil")
	}

	expectedMsg := "token missing expiration claim"
	if err.Error() != expectedMsg {
		t.Errorf("GetToken() error = %q, want %q", err.Error(), expectedMsg)
	}
}

func TestTokenManager_GetToken_ConcurrentAccess(t *testing.T) {
	expTime := time.Now().Add(1 * time.Hour)
	testToken := createTestJWT(expTime)

	mock := &mockTokenServiceClient{
		exchangeFunc: func(ctx context.Context, req *connect.Request[authnv1.ExchangeTokenRequest]) (*connect.Response[authnv1.ExchangeTokenResponse], error) {
			// Add a small delay to simulate network latency
			time.Sleep(10 * time.Millisecond)
			return connect.NewResponse(&authnv1.ExchangeTokenResponse{
				AccessToken: testToken,
			}), nil
		},
	}

	tm := &TokenManager{
		apiKey:        "test-api-key",
		authnEndpoint: "http://test.example.com",
		client:        mock,
	}

	// Start multiple goroutines that all try to get a token simultaneously
	const numGoroutines = 10
	var wg sync.WaitGroup
	errChan := make(chan error, numGoroutines)

	for range numGoroutines {
		wg.Go(func() {
			token, err := tm.GetToken(context.Background())
			if err != nil {
				errChan <- err
				return
			}
			if token != testToken {
				errChan <- errors.New("unexpected token value")
			}
		})
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Errorf("Concurrent GetToken() error: %v", err)
	}
}
