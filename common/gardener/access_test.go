package gardener

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testKubeconfig is a minimal kubeconfig that accessFromKubeconfig can parse.
const testKubeconfig = `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://shoot.example.com:443
  name: shoot
contexts:
- context:
    cluster: shoot
    user: admin
  name: shoot
current-context: shoot
users:
- name: admin
  user:
    token: test-token
`

// fakeSource is a configurable shootAccessSource.
type fakeSource struct {
	calls atomic.Int32

	expiresIn time.Duration // lifetime of returned kubeconfigs
	failAfter int32         // calls beyond this many fail (0 = never fail)
	blockCtx  bool          // block in FindShoot until ctx is done
	release   chan struct{} // if non-nil, FindShoot blocks until closed
	called    chan struct{} // if non-nil, receives one buffered signal per call
}

func (f *fakeSource) FindShoot(ctx context.Context, clusterID string) (*gardencorev1beta1.Shoot, error) {
	n := f.calls.Add(1)
	if f.called != nil {
		select {
		case f.called <- struct{}{}:
		default:
		}
	}
	if f.blockCtx {
		<-ctx.Done()
		return nil, fmt.Errorf("fake blocked until: %w", ctx.Err())
	}
	if f.release != nil {
		<-f.release
	}
	if f.failAfter > 0 && n > f.failAfter {
		return nil, errors.New("gardener unavailable")
	}
	shoot := &gardencorev1beta1.Shoot{}
	shoot.Name = "shoot-" + clusterID
	shoot.Labels = map[string]string{LabelOrganizationID: "org-1"}
	return shoot, nil
}

func (f *fakeSource) AdminKubeconfigForShoot(ctx context.Context, shoot *gardencorev1beta1.Shoot, expirationSeconds int64) (*AdminKubeconfig, error) {
	return &AdminKubeconfig{
		Kubeconfig: []byte(testKubeconfig),
		ExpiresAt:  time.Now().Add(f.expiresIn),
	}, nil
}

func newTestCache(t *testing.T, src shootAccessSource) *AdminKubeconfigCache {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cache := newAdminKubeconfigCache(src, logger)
	t.Cleanup(cache.entries.Stop)
	return cache
}

// TestAccessForHardExpiry pins that repeated reads do not extend an entry's
// lifetime (touch-on-hit is disabled): once the kubeconfig hard-expires, the
// stale entry is no longer served even if it was read continuously.
func TestAccessForHardExpiry(t *testing.T) {
	t.Parallel()

	fake := &fakeSource{expiresIn: 60 * time.Millisecond, failAfter: 1}
	cache := newTestCache(t, fake)

	access, err := cache.AccessFor(context.Background(), "c1")
	require.NoError(t, err)
	require.NotNil(t, access)

	// Read continuously; every fetch after the first fails, so a success can
	// only come from the cache. With touch-on-hit the entry would never
	// expire and this loop would never see an error.
	deadline := time.Now().Add(500 * time.Millisecond)
	var gotErr error
	for time.Now().Before(deadline) {
		_, err := cache.AccessFor(context.Background(), "c1")
		if err != nil {
			gotErr = err
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	require.Error(t, gotErr, "entry never hard-expired despite continuous reads")
	assert.ErrorContains(t, gotErr, "gardener unavailable")
}

// TestAccessForExpiredKubeconfig pins that an already-expired admin
// kubeconfig is rejected instead of being cached forever (ttlcache treats
// ttl <= 0 as never-expires).
func TestAccessForExpiredKubeconfig(t *testing.T) {
	t.Parallel()

	fake := &fakeSource{expiresIn: -time.Minute}
	cache := newTestCache(t, fake)

	_, err := cache.AccessFor(context.Background(), "c1")
	require.Error(t, err)
	assert.ErrorContains(t, err, "already expired")
	assert.Nil(t, cache.entries.Get("c1"), "expired credential must not be cached")
}

// TestAccessForMissTimeout pins that the synchronous miss path is bounded by
// fetchTimeout even though it is detached from the caller's context.
func TestAccessForMissTimeout(t *testing.T) {
	t.Parallel()

	fake := &fakeSource{blockCtx: true}
	cache := newTestCache(t, fake)
	cache.fetchTimeout = 50 * time.Millisecond

	start := time.Now()
	_, err := cache.AccessFor(context.Background(), "c1")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 2*time.Second)
}

// TestAccessForSingleflight pins that concurrent misses for the same cluster
// are deduplicated into a single fetch.
func TestAccessForSingleflight(t *testing.T) {
	t.Parallel()

	fake := &fakeSource{expiresIn: time.Hour, release: make(chan struct{})}
	cache := newTestCache(t, fake)

	const n = 10
	results := make([]*ShootAccess, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = cache.AccessFor(context.Background(), "c1")
		}()
	}

	// Give all goroutines time to join the in-flight fetch, then unblock it.
	time.Sleep(50 * time.Millisecond)
	close(fake.release)
	wg.Wait()

	assert.Equal(t, int32(1), fake.calls.Load(), "concurrent misses must share one fetch")
	for i := range n {
		require.NoError(t, errs[i])
		assert.Same(t, results[0], results[i])
	}
}

// TestAccessForCallerCancel pins that a caller whose context is cancelled is
// released immediately while the shared fetch keeps running and populates the
// cache for the next request.
func TestAccessForCallerCancel(t *testing.T) {
	t.Parallel()

	fake := &fakeSource{expiresIn: time.Hour, release: make(chan struct{})}
	cache := newTestCache(t, fake)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := cache.AccessFor(ctx, "c1")
		done <- err
	}()

	// Let the goroutine join the in-flight fetch, then abandon the request.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("cancelled caller stayed blocked on the shared fetch")
	}

	close(fake.release)
	require.Eventually(t, func() bool {
		return cache.entries.Get("c1") != nil
	}, 2*time.Second, 5*time.Millisecond, "shared fetch must outlive the cancelled caller and populate the cache")
}

// TestAccessForRefreshGuard pins that while a background refresh is stuck
// (e.g. Gardener unavailable), requests past refreshAt do not each spawn
// another refresh goroutine. Deliberately not parallel: it compares
// runtime.NumGoroutine before/after.
func TestAccessForRefreshGuard(t *testing.T) {
	fake := &fakeSource{expiresIn: time.Hour, release: make(chan struct{})}
	cache := newTestCache(t, fake)

	stale := &ShootAccess{refreshAt: time.Now().Add(-time.Minute)}
	cache.entries.Set("c1", stale, time.Hour)

	_, err := cache.AccessFor(context.Background(), "c1")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, busy := cache.refreshing.Load("c1")
		return busy
	}, time.Second, time.Millisecond, "first request past refreshAt must register a refresh")

	before := runtime.NumGoroutine()
	for range 100 {
		_, err := cache.AccessFor(context.Background(), "c1")
		require.NoError(t, err)
	}
	assert.Less(t, runtime.NumGoroutine(), before+20,
		"requests past refreshAt must not each park a refresh goroutine")

	close(fake.release)
	require.Eventually(t, func() bool {
		item := cache.entries.Get("c1")
		return item != nil && item.Value() != stale
	}, 2*time.Second, 5*time.Millisecond, "refresh never replaced the entry")
}

// TestAccessForBackgroundRefresh pins that once refreshAt has passed, the
// cached entry is still served immediately while a refresh runs in the
// background and eventually replaces it.
func TestAccessForBackgroundRefresh(t *testing.T) {
	t.Parallel()

	fake := &fakeSource{expiresIn: time.Hour, called: make(chan struct{}, 1)}
	cache := newTestCache(t, fake)

	stale := &ShootAccess{refreshAt: time.Now().Add(-time.Minute)}
	cache.entries.Set("c1", stale, time.Hour)

	got, err := cache.AccessFor(context.Background(), "c1")
	require.NoError(t, err)
	assert.Same(t, stale, got, "still-valid cached entry must be served, not blocked on the refresh")

	select {
	case <-fake.called:
	case <-time.After(2 * time.Second):
		t.Fatal("background refresh was not triggered")
	}

	require.Eventually(t, func() bool {
		item := cache.entries.Get("c1")
		return item != nil && item.Value() != stale
	}, 2*time.Second, 5*time.Millisecond, "background refresh never replaced the entry")
}
