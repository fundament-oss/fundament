package identity

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authnv1 "github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1"
	"github.com/fundament-oss/fundament/authn-api/pkg/proto/gen/authn/v1/authnv1connect"
)

// stubAuthn answers ExchangeWorkloadToken with a counter-stamped token.
type stubAuthn struct {
	authnv1connect.UnimplementedTokenServiceHandler
	calls     atomic.Int32
	expiresIn int64
	fail      atomic.Bool
	// gate, when set, holds every exchange until it is closed.
	gate     chan struct{}
	mu       sync.Mutex
	lastAuth string
	lastID   string
}

func (s *stubAuthn) ExchangeWorkloadToken(ctx context.Context, req *authnv1.ExchangeWorkloadTokenRequest) (*authnv1.ExchangeWorkloadTokenResponse, error) {
	n := s.calls.Add(1)
	if s.gate != nil {
		<-s.gate
	}
	callInfo, _ := connect.CallInfoForHandlerContext(ctx)
	s.mu.Lock()
	s.lastAuth = callInfo.RequestHeader().Get("Authorization")
	s.lastID = req.GetClusterId()
	s.mu.Unlock()
	if s.fail.Load() {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid workload credential"))
	}
	return authnv1.ExchangeWorkloadTokenResponse_builder{
		AccessToken: "workload-token-" + strconv.Itoa(int(n)),
		TokenType:   "Bearer",
		ExpiresIn:   s.expiresIn,
	}.Build(), nil
}

func newHarness(t *testing.T, expiresIn int64) (*stubAuthn, *FileTokenSource, string) {
	t.Helper()
	stub := &stubAuthn{expiresIn: expiresIn}
	mux := http.NewServeMux()
	mux.Handle(authnv1connect.NewTokenServiceHandler(stub))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	path := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(path, []byte("projected-1\n"), 0o600))

	client := authnv1connect.NewTokenServiceClient(srv.Client(), srv.URL)
	src := NewFileTokenSource(path, "019b4000-2000-7000-8000-000000000001", client, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return stub, src, path
}

func TestFileTokenSource_ExchangesAndCaches(t *testing.T) {
	stub, src, _ := newHarness(t, 900)

	tok, err := src.Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "workload-token-1", tok)
	assert.Equal(t, "Bearer projected-1", stub.lastAuth, "the file's contents, trimmed, are the credential")
	assert.Equal(t, "019b4000-2000-7000-8000-000000000001", stub.lastID)

	tok, err = src.Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "workload-token-1", tok)
	assert.Equal(t, int32(1), stub.calls.Load(), "a fresh token is served from cache")
}

func TestFileTokenSource_RefreshesAt80PercentAndRereadsFile(t *testing.T) {
	stub, src, path := newHarness(t, 1000)
	now := time.Now()
	src.now = func() time.Time { return now }

	_, err := src.Token(context.Background())
	require.NoError(t, err)

	// kubelet rotated the projected token in the meantime.
	require.NoError(t, os.WriteFile(path, []byte("projected-2"), 0o600))

	now = now.Add(799 * time.Second)
	tok, err := src.Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "workload-token-1", tok, "before 80% the cached token is served")

	now = now.Add(2 * time.Second)
	tok, err = src.Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "workload-token-1", tok, "past 80% the cached token is still served while the refresh runs")
	src.background.Wait()
	tok, err = src.Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "workload-token-2", tok, "the background refresh replaced it")
	assert.Equal(t, "Bearer projected-2", stub.lastAuth, "the rotated file is read on every exchange")
}

func TestFileTokenSource_ServesCachedTokenWhenRefreshFails(t *testing.T) {
	stub, src, _ := newHarness(t, 1000)
	now := time.Now()
	src.now = func() time.Time { return now }

	_, err := src.Token(context.Background())
	require.NoError(t, err)

	stub.fail.Store(true)
	now = now.Add(900 * time.Second)
	tok, err := src.Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "workload-token-1", tok, "still valid, so the failed refresh is not the caller's problem")
	src.background.Wait()

	now = now.Add(200 * time.Second)
	_, err = src.Token(context.Background())
	require.Error(t, err, "expired and not refreshable")
	assert.Contains(t, err.Error(), "invalid workload credential")
}

func TestFileTokenSource_MissingFile(t *testing.T) {
	_, src, path := newHarness(t, 900)
	require.NoError(t, os.Remove(path))

	_, err := src.Token(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read projected token")
}

func TestFileTokenSource_ConcurrentCallersShareOneExchange(t *testing.T) {
	stub, src, _ := newHarness(t, 900)

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			tok, err := src.Token(context.Background())
			assert.NoError(t, err)
			assert.Equal(t, "workload-token-1", tok)
		})
	}
	wg.Wait()
	assert.Equal(t, int32(1), stub.calls.Load())
}

func TestBearerInterceptor(t *testing.T) {
	var seen string
	next := connect.UnaryFunc(func(_ context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		seen = req.Header().Get("Authorization")
		return nil, nil //nolint:nilnil // stub
	})
	req := connect.NewRequest(&authnv1.ExchangeWorkloadTokenRequest{})

	_, err := BearerInterceptor(StaticTokenSource("tok"))(next)(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "Bearer tok", seen)

	failing := tokenSourceFunc(func(context.Context) (string, error) { return "", errors.New("no token") })
	_, err = BearerInterceptor(failing)(next)(context.Background(), req)
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

type tokenSourceFunc func(context.Context) (string, error)

func (f tokenSourceFunc) Token(ctx context.Context) (string, error) { return f(ctx) }

func TestFileTokenSource_FailedRefreshBacksOff(t *testing.T) {
	stub, src, _ := newHarness(t, 1000)
	now := time.Now()
	src.now = func() time.Time { return now }

	_, err := src.Token(context.Background())
	require.NoError(t, err)

	stub.fail.Store(true)
	now = now.Add(850 * time.Second)
	_, err = src.Token(context.Background())
	require.NoError(t, err)
	src.background.Wait()
	require.Equal(t, int32(2), stub.calls.Load())

	now = now.Add(refreshRetryInterval - time.Second)
	tok, err := src.Token(context.Background())
	require.NoError(t, err)
	src.background.Wait()
	assert.Equal(t, "workload-token-1", tok)
	assert.Equal(t, int32(2), stub.calls.Load(), "no retry inside the back-off window")

	stub.fail.Store(false)
	now = now.Add(2 * time.Second)
	tok, err = src.Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "workload-token-1", tok, "the retry runs in the background")
	src.background.Wait()
	tok, err = src.Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "workload-token-3", tok, "retried once the window passed")
}

func TestFileTokenSource_RefreshInFlightServesCachedToken(t *testing.T) {
	stub, src, _ := newHarness(t, 1000)
	now := time.Now()
	src.now = func() time.Time { return now }

	_, err := src.Token(context.Background())
	require.NoError(t, err)

	stub.gate = make(chan struct{})
	now = now.Add(850 * time.Second)
	tok, err := src.Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "workload-token-1", tok, "the caller that starts the refresh does not wait for it")
	require.Eventually(t, func() bool { return stub.calls.Load() == 2 }, time.Second, time.Millisecond)

	tok, err = src.Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "workload-token-1", tok, "callers do not queue behind a refresh")
	assert.Equal(t, int32(2), stub.calls.Load(), "one refresh at a time")

	close(stub.gate)
	src.background.Wait()
	tok, err = src.Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "workload-token-2", tok)
}

// The refresh runs on its own context: a caller that is already cancelled
// (or nearly out of deadline) starts it without failing it.
func TestFileTokenSource_RefreshSurvivesCallerCancellation(t *testing.T) {
	_, src, _ := newHarness(t, 1000)
	now := time.Now()
	src.now = func() time.Time { return now }

	_, err := src.Token(context.Background())
	require.NoError(t, err)

	now = now.Add(850 * time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tok, err := src.Token(ctx)
	require.NoError(t, err)
	assert.Equal(t, "workload-token-1", tok)
	src.background.Wait()

	tok, err = src.Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "workload-token-2", tok, "the refresh succeeded despite its caller's cancelled context")
}
