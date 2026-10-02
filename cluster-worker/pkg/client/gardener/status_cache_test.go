package gardener

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// fakeStatusCache is a fake client with the cluster-ID index, standing in for
// the controller-runtime cache.
type fakeStatusCache struct {
	client.Client
	indexed *bool
}

// IndexField records the call; the fake client got the index from its builder.
func (f fakeStatusCache) IndexField(context.Context, client.Object, string, client.IndexerFunc) error {
	if f.indexed != nil {
		*f.indexed = true
	}
	return nil
}

func (fakeStatusCache) Start(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (fakeStatusCache) WaitForCacheSync(context.Context) bool { return true }

// newCachedStatusTestClient returns a RealClient whose direct reads see
// directShoot and whose cache holds cachedShoot, so a test can tell which
// source a status read used.
func newCachedStatusTestClient(t *testing.T, directShoot, cachedShoot *gardencorev1beta1.Shoot) *RealClient {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, gardencorev1beta1.AddToScheme(scheme))

	direct := fake.NewClientBuilder().WithScheme(scheme).WithObjects(directShoot).Build()
	cached := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gardencorev1beta1.Shoot{}, shootClusterIDIndex, shootClusterIDValue).
		WithObjects(cachedShoot).
		Build()

	return &RealClient{
		client:      direct,
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		statusCache: fakeStatusCache{Client: cached},
	}
}

func TestGetShootStatusReadsFromCache(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	recentProbe := now.Add(-5 * time.Second)
	tests := []struct {
		name        string
		synced      bool
		lastProbe   time.Time
		lastFailure time.Time
		wantCache   bool
	}{
		{name: "synced, probe fresh, watch healthy", synced: true, lastProbe: recentProbe, wantCache: true},
		{name: "not synced yet", synced: false, lastProbe: recentProbe, wantCache: false},
		{name: "no probe answered yet", synced: true, wantCache: false},
		{name: "last probe answer too old (garden hangs)", synced: true, lastProbe: now.Add(-40 * time.Second), wantCache: false},
		{name: "watch failed 10s ago", synced: true, lastProbe: recentProbe, lastFailure: now.Add(-10 * time.Second), wantCache: false},
		{name: "watch failed 2m ago", synced: true, lastProbe: recentProbe, lastFailure: now.Add(-2 * time.Minute), wantCache: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clusterID := uuid.New()
			direct := statusTestShoot(clusterID, lastOp(gardencorev1beta1.LastOperationTypeCreate, gardencorev1beta1.LastOperationStateProcessing, "direct"))
			cached := statusTestShoot(clusterID, lastOp(gardencorev1beta1.LastOperationTypeCreate, gardencorev1beta1.LastOperationStateSucceeded, "cached"), healthyConditions()...)
			r := newCachedStatusTestClient(t, direct, cached)
			r.clock = func() time.Time { return now }
			r.statusCacheSynced.Store(tt.synced)
			if !tt.lastProbe.IsZero() {
				r.lastProbeSuccess.Store(tt.lastProbe.UnixNano())
			}
			if !tt.lastFailure.IsZero() {
				r.lastWatchFailure.Store(tt.lastFailure.UnixNano())
			}

			got, err := r.GetShootStatus(t.Context(), &ClusterToSync{ID: clusterID})
			require.NoError(t, err)

			if tt.wantCache {
				assert.Equal(t, StatusReady, got.Status)
			} else {
				assert.Equal(t, StatusProgressing, got.Status)
			}
		})
	}
}

func TestRunStatusCacheIndexesStartsAndSyncs(t *testing.T) {
	t.Parallel()

	shoot := statusTestShoot(uuid.New(), nil)
	r := newCachedStatusTestClient(t, shoot, shoot)
	indexed := false
	r.statusCache = fakeStatusCache{Client: r.statusCache.(fakeStatusCache).Client, indexed: &indexed}

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- r.RunStatusCache(ctx) }()

	// Usable needs both the first sync and the first successful garden probe.
	require.Eventually(t, r.statusCacheUsable, time.Second, 10*time.Millisecond)
	assert.True(t, indexed)

	cancel()
	require.NoError(t, <-done)
}

func TestProbeGardenOnce(t *testing.T) {
	t.Parallel()

	shoot := statusTestShoot(uuid.New(), nil)

	t.Run("answer keeps the cache usable", func(t *testing.T) {
		t.Parallel()
		r := newCachedStatusTestClient(t, shoot, shoot)
		r.statusCacheSynced.Store(true)

		r.probeGardenOnce(t.Context())

		assert.NotZero(t, r.lastProbeSuccess.Load())
		assert.Zero(t, r.lastWatchFailure.Load())
		assert.True(t, r.statusCacheUsable())
	})

	t.Run("failure forces direct reads", func(t *testing.T) {
		t.Parallel()
		r := newCachedStatusTestClient(t, shoot, shoot)
		r.statusCacheSynced.Store(true)
		r.lastProbeSuccess.Store(time.Now().UnixNano())
		scheme := runtime.NewScheme()
		require.NoError(t, gardencorev1beta1.AddToScheme(scheme))
		r.client = fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
			List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
				return errors.New("garden unreachable")
			},
		}).Build()

		r.probeGardenOnce(t.Context())

		assert.NotZero(t, r.lastWatchFailure.Load())
		assert.False(t, r.statusCacheUsable())
	})
}

func TestGetShootStatusCacheMissIsNotFound(t *testing.T) {
	t.Parallel()

	other := statusTestShoot(uuid.New(), lastOp(gardencorev1beta1.LastOperationTypeCreate, gardencorev1beta1.LastOperationStateSucceeded, ""), healthyConditions()...)
	r := newCachedStatusTestClient(t, other, other)
	r.statusCacheSynced.Store(true)
	r.lastProbeSuccess.Store(time.Now().UnixNano())

	got, err := r.GetShootStatus(t.Context(), &ClusterToSync{ID: uuid.New()})
	require.NoError(t, err)
	assert.Equal(t, StatusPending, got.Status)
	assert.Equal(t, MsgShootNotFound, got.Message)
}

func TestIsWatchFailure(t *testing.T) {
	t.Parallel()

	shoots := schema.GroupResource{Group: "core.gardener.cloud", Resource: "shoots"}
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "watch closed normally", err: io.EOF, want: false},
		{name: "resource version expired", err: apierrors.NewResourceExpired("too old"), want: false},
		{name: "unauthorized", err: apierrors.NewUnauthorized("token expired"), want: true},
		{name: "forbidden", err: apierrors.NewForbidden(shoots, "", errors.New("no watch")), want: true},
		{name: "connection refused", err: fmt.Errorf("dial: %w", errors.New("connection refused")), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isWatchFailure(tt.err))
		})
	}
}

func TestTrimShootForStatus(t *testing.T) {
	t.Parallel()

	clusterID := uuid.New()
	deleting := metav1.Now()
	shoot := statusTestShoot(clusterID, lastOp(gardencorev1beta1.LastOperationTypeReconcile, gardencorev1beta1.LastOperationStateSucceeded, "done"), healthyConditions()...)
	shoot.DeletionTimestamp = &deleting
	shoot.ManagedFields = []metav1.ManagedFieldsEntry{{Manager: "gardenlet"}}
	shoot.Spec.Kubernetes.Version = "1.34.0"

	got, err := trimShootForStatus(shoot)
	require.NoError(t, err)
	trimmed, ok := got.(*gardencorev1beta1.Shoot)
	require.True(t, ok)

	assert.Empty(t, trimmed.ManagedFields)
	assert.Empty(t, trimmed.Spec.Kubernetes.Version)
	assert.Equal(t, clusterID.String(), trimmed.Labels[LabelClusterID])
	assert.NotNil(t, trimmed.DeletionTimestamp)
	assert.Equal(t, gardencorev1beta1.LastOperationStateSucceeded, trimmed.Status.LastOperation.State)
	assert.Len(t, trimmed.Status.Conditions, 3)
}

func TestShootClusterIDValue(t *testing.T) {
	t.Parallel()

	clusterID := uuid.New()
	assert.Equal(t, []string{clusterID.String()}, shootClusterIDValue(statusTestShoot(clusterID, nil)))
	assert.Nil(t, shootClusterIDValue(&gardencorev1beta1.Shoot{}))
}
