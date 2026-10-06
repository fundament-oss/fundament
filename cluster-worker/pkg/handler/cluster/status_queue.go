package cluster

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"k8s.io/client-go/util/workqueue"
)

const (
	// A failed status check is retried with exponential backoff from
	// statusRetryBaseDelay up to statusRetryMaxDelay, at most
	// statusMaxRetries times; after that the next poll picks the cluster up.
	statusRetryBaseDelay = time.Second
	statusRetryMaxDelay  = 5 * time.Minute
	statusMaxRetries     = 10
)

// newStatusQueue returns the queue of clusters whose status needs a check. It
// de-duplicates: a cluster queued several times before a worker takes it is
// checked once, and never by two workers at the same time.
func newStatusQueue() workqueue.TypedRateLimitingInterface[uuid.UUID] {
	return workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.NewTypedItemExponentialFailureRateLimiter[uuid.UUID](statusRetryBaseDelay, statusRetryMaxDelay),
		workqueue.TypedRateLimitingQueueConfig[uuid.UUID]{Name: "cluster-status"},
	)
}

// EnqueueStatusCheck queues a status check for one cluster.
func (h *Handler) EnqueueStatusCheck(clusterID uuid.UUID) {
	h.statusQueue.Add(clusterID)
}

// RunStatusWorkers checks queued clusters with StatusWorkers workers until ctx
// is done, then lets the checks in progress finish.
func (h *Handler) RunStatusWorkers(ctx context.Context) error {
	workers := max(h.cfg.StatusWorkers, 1)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for h.processNextStatusCheck(ctx) {
			}
		})
	}
	<-ctx.Done()
	h.statusQueue.ShutDown()
	wg.Wait()
	return nil
}

// processNextStatusCheck runs one queued check. It returns false once the
// queue is shut down.
func (h *Handler) processNextStatusCheck(ctx context.Context) bool {
	clusterID, shutdown := h.statusQueue.Get()
	if shutdown {
		return false
	}
	defer h.statusQueue.Done(clusterID)

	err := h.CheckCluster(ctx, clusterID)
	if err == nil || ctx.Err() != nil {
		h.statusQueue.Forget(clusterID)
		return true
	}
	if h.statusQueue.NumRequeues(clusterID) < statusMaxRetries {
		h.logger.Warn("status check failed, retrying", "cluster_id", clusterID, "error", err)
		h.statusQueue.AddRateLimited(clusterID)
		return true
	}
	h.logger.Error("status check failed, giving up until the next poll", "cluster_id", clusterID, "error", err)
	h.statusQueue.Forget(clusterID)
	return true
}
