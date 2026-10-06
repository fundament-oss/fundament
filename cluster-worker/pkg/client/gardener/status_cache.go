package gardener

// Shoot status cache. One watch on Fundament-labelled Shoots keeps their status
// in memory, so a status poll costs no garden request, however many clusters
// there are. Only GetShootStatus reads from it; writes always use the direct
// client, because the cached Shoots have their spec trimmed away. The same
// watch reports status changes (SetStatusChangeHandler), so a change reaches
// the database without waiting for a poll.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// shootClusterIDIndex indexes cached Shoots by their cluster-ID label.
	shootClusterIDIndex = "fundament.io/cluster-id"

	// watchFailureFallback is how long after a real watch failure status reads
	// go to Gardener directly. While the garden is unreachable the cache keeps
	// serving its last state; reading through means an outage shows up as
	// errors, as it did before the cache, instead of as silently stale status.
	watchFailureFallback = time.Minute

	// indexRetryInterval is how long RunStatusCache waits before retrying the
	// index registration, which needs the garden API to resolve the Shoot type.
	indexRetryInterval = 30 * time.Second

	// A watch on a garden that hangs instead of refusing connections never
	// reports an error, so a cheap direct request probes the garden alongside
	// it. The cache is only used while a probe succeeded within probeStaleAfter,
	// which bounds how stale a status read can be to about that long.
	gardenProbeInterval = 15 * time.Second
	gardenProbeTimeout  = 10 * time.Second
	probeStaleAfter     = gardenProbeInterval + gardenProbeTimeout + 5*time.Second

	// statusReadTimeout bounds a direct status read, so a hung garden makes a
	// status poll fail instead of blocking it.
	statusReadTimeout = 10 * time.Second
)

// shootStatusCache is the part of a controller-runtime cache the status path uses.
type shootStatusCache interface {
	client.Reader
	client.FieldIndexer
	GetInformer(ctx context.Context, obj client.Object, opts ...cache.InformerGetOption) (cache.Informer, error)
	Start(ctx context.Context) error
	WaitForCacheSync(ctx context.Context) bool
}

func newShootStatusCache(cfg *rest.Config, scheme *runtime.Scheme, onWatchError toolscache.WatchErrorHandlerWithContext) (cache.Cache, error) {
	hasClusterID, err := labels.NewRequirement(LabelClusterID, selection.Exists, nil)
	if err != nil {
		return nil, fmt.Errorf("cluster-id label selector: %w", err)
	}

	c, err := cache.New(cfg, cache.Options{
		Scheme: scheme,
		ByObject: map[client.Object]cache.ByObject{
			&gardencorev1beta1.Shoot{}: {
				Label:     labels.NewSelector().Add(*hasClusterID),
				Transform: trimShootForStatus,
			},
		},
		ReaderFailOnMissingInformer: true,
		DefaultWatchErrorHandler:    onWatchError,
	})
	if err != nil {
		return nil, fmt.Errorf("create shoot status cache: %w", err)
	}
	return c, nil
}

// trimShootForStatus keeps what GetShootStatus reads (metadata, status) and
// drops the spec and managed fields, which are most of a Shoot's size.
func trimShootForStatus(obj any) (any, error) {
	shoot, ok := obj.(*gardencorev1beta1.Shoot)
	if !ok {
		return obj, nil
	}
	shoot.ManagedFields = nil
	shoot.Spec = gardencorev1beta1.ShootSpec{}
	return shoot, nil
}

func shootClusterIDValue(obj client.Object) []string {
	if id := obj.GetLabels()[LabelClusterID]; id != "" {
		return []string{id}
	}
	return nil
}

// RunStatusCache runs the shoot status cache until ctx is done. Status reads
// go to Gardener directly until its first sync completes. An unreachable
// garden delays the cache instead of failing startup: registering the index
// (which also registers the Shoot informer) is retried until it succeeds.
func (r *RealClient) RunStatusCache(ctx context.Context) error {
	if r.statusCache == nil {
		return nil
	}

	indexShoots := func() error {
		return r.statusCache.IndexField(ctx, &gardencorev1beta1.Shoot{}, shootClusterIDIndex, shootClusterIDValue)
	}
	if !r.retryUntilGardenAnswers(ctx, indexShoots) {
		return nil
	}
	// The index registered the Shoot informer, so the handler goes on before
	// the cache starts and sees every Shoot of the first load. Only getting
	// the informer needs the garden; the handler is added exactly once, since
	// a second registration would report every change twice.
	if r.onStatusChange != nil {
		var informer cache.Informer
		getInformer := func() error {
			var err error
			informer, err = r.statusCache.GetInformer(ctx, &gardencorev1beta1.Shoot{})
			if err != nil {
				return fmt.Errorf("get shoot informer: %w", err)
			}
			return nil
		}
		if !r.retryUntilGardenAnswers(ctx, getInformer) {
			return nil
		}
		if err := r.addStatusChangeHandler(informer); err != nil {
			return err
		}
	}

	go func() {
		if r.statusCache.WaitForCacheSync(ctx) {
			r.statusCacheSynced.Store(true)
			r.logger.Info("shoot status cache synced")
		}
	}()
	go r.probeGarden(ctx)

	if err := r.statusCache.Start(ctx); err != nil {
		return fmt.Errorf("run shoot status cache: %w", err)
	}
	return nil
}

// retryUntilGardenAnswers runs step until it succeeds; registering with the
// cache needs the garden API to resolve the Shoot type. It returns false when
// ctx ends first.
func (r *RealClient) retryUntilGardenAnswers(ctx context.Context, step func() error) bool {
	for {
		err := step()
		if err == nil {
			return true
		}
		r.logger.Warn("shoot status cache not started yet, retrying", "error", err, "retry_in", indexRetryInterval)
		select {
		case <-ctx.Done():
			return false
		case <-time.After(indexRetryInterval):
		}
	}
}

// SetStatusChangeHandler registers fn to receive the cluster ID of every Shoot
// whose derived status changes in the watch, plus every Shoot once when the
// cache loads and every Shoot that is deleted. Call it before RunStatusCache;
// fn must not block.
func (r *RealClient) SetStatusChangeHandler(fn func(clusterID uuid.UUID)) {
	r.onStatusChange = fn
}

// addStatusChangeHandler registers the status change handler on the Shoot
// informer. Adding a handler only fails on a stopped informer, so the error
// is final.
func (r *RealClient) addStatusChangeHandler(informer cache.Informer) error {
	if _, err := informer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc: r.notifyStatusChange,
		UpdateFunc: func(oldObj, newObj any) {
			if shootStatusChanged(oldObj, newObj) {
				r.notifyStatusChange(newObj)
			}
		},
		DeleteFunc: func(obj any) { r.notifyStatusChange(unwrapTombstone(obj)) },
	}); err != nil {
		return fmt.Errorf("add shoot status handler: %w", err)
	}
	return nil
}

// shootStatusChanged reports whether an update changes the status fundament
// derives. Gardener rewrites a Shoot far more often than that: progress after
// every reconcile task, condition messages, and the cache's periodic resync,
// which passes the same object twice.
func shootStatusChanged(oldObj, newObj any) bool {
	oldShoot, oldOK := oldObj.(*gardencorev1beta1.Shoot)
	newShoot, newOK := newObj.(*gardencorev1beta1.Shoot)
	if !oldOK || !newOK {
		return true
	}
	return *shootStatusOf(oldShoot) != *shootStatusOf(newShoot)
}

// unwrapTombstone returns the last known object of a delete the watch missed
// and only noticed on a re-list.
func unwrapTombstone(obj any) any {
	if tombstone, ok := obj.(toolscache.DeletedFinalStateUnknown); ok {
		return tombstone.Obj
	}
	return obj
}

func (r *RealClient) notifyStatusChange(obj any) {
	shoot, ok := obj.(*gardencorev1beta1.Shoot)
	if !ok {
		return
	}
	clusterID, err := uuid.Parse(shoot.Labels[LabelClusterID])
	if err != nil {
		return
	}
	r.onStatusChange(clusterID)
}

// recordWatchError keeps client-go's default logging and remembers when the
// watch last failed for real. Normal watch closes and expired resource
// versions are routine: the reflector reconnects or re-lists by itself.
func (r *RealClient) recordWatchError(ctx context.Context, reflector *toolscache.Reflector, err error) {
	toolscache.DefaultWatchErrorHandler(ctx, reflector, err)
	if !isWatchFailure(err) {
		return
	}
	r.lastWatchFailure.Store(r.now().UnixNano())
	r.logger.Warn("shoot status watch failing, status polls read Gardener directly", "error", err)
}

func isWatchFailure(err error) bool {
	return !errors.Is(err, io.EOF) && !apierrors.IsResourceExpired(err) && !apierrors.IsGone(err)
}

// probeGarden checks every gardenProbeInterval that the garden answers.
func (r *RealClient) probeGarden(ctx context.Context) {
	ticker := time.NewTicker(gardenProbeInterval)
	defer ticker.Stop()
	for {
		r.probeGardenOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// probeGardenOnce makes one bounded request to the garden. Success keeps the
// cache usable; a failure counts as a watch failure.
func (r *RealClient) probeGardenOnce(ctx context.Context) {
	probeCtx, cancel := context.WithTimeout(ctx, gardenProbeTimeout)
	defer cancel()

	err := r.client.List(probeCtx, &gardencorev1beta1.ShootList{}, client.HasLabels{LabelClusterID}, client.Limit(1))
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		r.lastWatchFailure.Store(r.now().UnixNano())
		r.logger.Warn("garden probe failed, status polls read Gardener directly", "error", err)
		return
	}
	r.lastProbeSuccess.Store(r.now().UnixNano())
}

// statusCacheUsable reports whether status reads can come from the cache: it
// has synced, the garden answered a probe recently, and the watch has not
// failed within watchFailureFallback.
func (r *RealClient) statusCacheUsable() bool {
	if r.statusCache == nil || !r.statusCacheSynced.Load() {
		return false
	}
	now := r.now()
	lastProbe := r.lastProbeSuccess.Load()
	if lastProbe == 0 || now.Sub(time.Unix(0, lastProbe)) >= probeStaleAfter {
		return false
	}
	lastFailure := r.lastWatchFailure.Load()
	return lastFailure == 0 || now.Sub(time.Unix(0, lastFailure)) >= watchFailureFallback
}

// getShootForStatus finds a Shoot by cluster ID for the status path: from the
// cache when it is usable, otherwise from Gardener directly. Returns nil if
// not found.
func (r *RealClient) getShootForStatus(ctx context.Context, clusterID uuid.UUID) (*gardencorev1beta1.Shoot, error) {
	if !r.statusCacheUsable() {
		readCtx, cancel := context.WithTimeout(ctx, statusReadTimeout)
		defer cancel()
		return r.getShootByClusterID(readCtx, clusterID)
	}

	shootList := &gardencorev1beta1.ShootList{}
	if err := r.statusCache.List(ctx, shootList, client.MatchingFields{shootClusterIDIndex: clusterID.String()}); err != nil {
		return nil, fmt.Errorf("list cached shoots: %w", err)
	}
	if len(shootList.Items) == 0 {
		return nil, nil
	}
	if len(shootList.Items) > 1 {
		r.logger.Warn("multiple shoots found for cluster ID",
			"cluster_id", clusterID,
			"count", len(shootList.Items))
	}
	return &shootList.Items[0], nil
}

func (r *RealClient) now() time.Time {
	if r.clock != nil {
		return r.clock()
	}
	return time.Now()
}
