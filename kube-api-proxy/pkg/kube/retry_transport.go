package kube

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/kube-api-proxy/pkg/token"
)

// retryTransport wraps an http.RoundTripper to handle 401 responses by
// refreshing the SA token and retrying the request (GET only).
type retryTransport struct {
	inner      http.RoundTripper
	tokenCache *token.Cache
	logger     *slog.Logger
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.inner.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("round trip: %w", err)
	}

	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	// Extract user and cluster from context to refresh the token.
	userID, userOK := req.Context().Value(UserIDContextKey{}).(uuid.UUID)
	clusterID, clusterOK := req.Context().Value(ClusterIDContextKey{}).(string)
	if !userOK || !clusterOK {
		return resp, nil
	}

	// Force-refresh the cached token for future requests.
	newToken, refreshErr := t.tokenCache.ForceRefresh(req.Context(), userID, clusterID)
	if refreshErr != nil {
		t.logger.WarnContext(req.Context(), "401 token refresh failed", "error", refreshErr)
		return resp, nil
	}

	// Only retry GET requests — mutating requests may have already streamed the body.
	if req.Method != http.MethodGet {
		return resp, nil
	}

	// Close the original 401 response body before retrying.
	_ = resp.Body.Close()

	// Retry with the new token.
	req.Header.Set("Authorization", "Bearer "+newToken)
	retryResp, retryErr := t.inner.RoundTrip(req)
	if retryErr != nil {
		return nil, fmt.Errorf("retry round trip: %w", retryErr)
	}
	return retryResp, nil
}
