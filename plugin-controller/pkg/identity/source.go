// Package identity turns the projected ServiceAccount token kubelet keeps on
// disk into a fundament WorkloadToken (FUN-22, "The first consumer"). The
// controller never holds a static fundament secret: the credential on disk
// is minted by its own cluster, rotated by kubelet, and exchanged at authn-api
// for a short-lived token bound to this cluster and organization.
package identity

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"

	authnv1 "github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1"
	"github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1/authnv1connect"
)

const (
	// refreshRatio is the fraction of a WorkloadToken's lifetime after which
	// the next Token call re-exchanges. Mirrors kube-api-proxy's token cache.
	refreshRatio = 0.8
	// refreshTimeout bounds a proactive refresh, so the caller that runs it
	// keeps most of its own deadline for the call the token is for.
	refreshTimeout = 5 * time.Second
	// refreshRetryInterval is how long a failed refresh is not retried while
	// the cached token stays valid.
	refreshRetryInterval = 10 * time.Second
)

// DefaultTokenFile is where the projected volume mounts the credential (see
// cluster-worker's manifests and the chart's plugin-controller Deployment).
const DefaultTokenFile = "/var/run/secrets/fundament/token" //nolint:gosec // a path, not a credential

// TokenSource yields a valid WorkloadToken for outbound calls.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// StaticTokenSource returns one fixed token; for tests and tooling.
type StaticTokenSource string

func (s StaticTokenSource) Token(context.Context) (string, error) { return string(s), nil }

// FileTokenSource reads the projected token from disk at every exchange (it
// rotates underneath the pod), exchanges it at authn-api for the named
// cluster, and caches the result until refreshRatio of its lifetime.
type FileTokenSource struct {
	path      string
	clusterID string
	client    authnv1connect.TokenServiceClient
	logger    *slog.Logger
	now       func() time.Time

	// exchangeMu serialises exchanges made without a usable token, so
	// concurrent first callers share one.
	exchangeMu sync.Mutex

	mu         sync.Mutex
	token      string
	issuedAt   time.Time
	expiresAt  time.Time
	refreshing bool
	retryAt    time.Time

	// background tracks the in-flight refresh, so tests can wait for it.
	background sync.WaitGroup
}

// NewFileTokenSource returns a FileTokenSource. Nothing is read or exchanged
// until the first Token call.
func NewFileTokenSource(path, clusterID string, client authnv1connect.TokenServiceClient, logger *slog.Logger) *FileTokenSource {
	return &FileTokenSource{path: path, clusterID: clusterID, client: client, logger: logger, now: time.Now}
}

// Token implements TokenSource. While the cached token is fresh it is served
// as is. Past refreshRatio of its lifetime the first caller starts one
// background re-exchange, on its own context and timeout rather than the
// caller's, and every caller keeps getting the cached token; a failed refresh
// logs and is not retried for refreshRetryInterval. Only a missing or expired
// token makes an exchange failure the caller's.
func (s *FileTokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	now := s.now()
	if s.token != "" && now.Before(s.expiresAt) {
		cached := s.token
		refreshAt := s.issuedAt.Add(time.Duration(float64(s.expiresAt.Sub(s.issuedAt)) * refreshRatio))
		if now.Before(refreshAt) || s.refreshing || now.Before(s.retryAt) {
			s.mu.Unlock()
			return cached, nil
		}
		s.refreshing = true
		s.mu.Unlock()
		s.background.Go(func() { s.refresh(context.WithoutCancel(ctx)) })
		return cached, nil
	}
	s.mu.Unlock()
	return s.exchangeWithoutToken(ctx)
}

func (s *FileTokenSource) refresh(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, refreshTimeout)
	defer cancel()
	token, lifetime, err := s.exchange(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.refreshing = false
	if err != nil {
		s.retryAt = s.now().Add(refreshRetryInterval)
		s.logger.WarnContext(ctx, "workload token refresh failed; serving the cached token until it expires",
			"error", err, "expires_at", s.expiresAt, "retry_at", s.retryAt)
		return
	}
	s.store(ctx, token, lifetime)
}

func (s *FileTokenSource) exchangeWithoutToken(ctx context.Context) (string, error) {
	s.exchangeMu.Lock()
	defer s.exchangeMu.Unlock()

	// Another caller may have exchanged while this one waited.
	s.mu.Lock()
	if s.token != "" && s.now().Before(s.expiresAt) {
		token := s.token
		s.mu.Unlock()
		return token, nil
	}
	s.mu.Unlock()

	token, lifetime, err := s.exchange(ctx)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store(ctx, token, lifetime)
	return s.token, nil
}

// store caches a fresh token; s.mu must be held.
func (s *FileTokenSource) store(ctx context.Context, token string, lifetime time.Duration) {
	now := s.now()
	s.token = token
	s.issuedAt = now
	s.expiresAt = now.Add(lifetime)
	s.retryAt = time.Time{}
	s.logger.DebugContext(ctx, "workload token exchanged", "cluster_id", s.clusterID, "expires_at", s.expiresAt)
}

func (s *FileTokenSource) exchange(ctx context.Context) (string, time.Duration, error) {
	credential, err := os.ReadFile(s.path)
	if err != nil {
		return "", 0, fmt.Errorf("read projected token %s: %w", s.path, err)
	}
	bearer := strings.TrimSpace(string(credential))
	if bearer == "" {
		return "", 0, fmt.Errorf("projected token %s is empty", s.path)
	}

	// The client context is created here for the nested call only: connect
	// prohibits changing call info mid-flight, so it must never flow into an
	// interceptor's next().
	callCtx, callInfo := connect.NewClientContext(ctx)
	callInfo.RequestHeader().Set("Authorization", "Bearer "+bearer)

	resp, err := s.client.ExchangeWorkloadToken(callCtx, authnv1.ExchangeWorkloadTokenRequest_builder{
		ClusterId: s.clusterID,
	}.Build())
	if err != nil {
		return "", 0, fmt.Errorf("exchange workload token for cluster %s: %w", s.clusterID, err)
	}
	if resp.GetAccessToken() == "" || resp.GetExpiresIn() <= 0 {
		return "", 0, fmt.Errorf("exchange workload token: empty token or lifetime in response")
	}
	return resp.GetAccessToken(), time.Duration(resp.GetExpiresIn()) * time.Second, nil
}

// BearerInterceptor adds the source's token to every outbound unary call.
func BearerInterceptor(source TokenSource) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			token, err := source.Token(ctx)
			if err != nil {
				return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("workload token: %w", err))
			}
			req.Header().Set("Authorization", "Bearer "+token)
			return next(ctx, req)
		}
	}
}
