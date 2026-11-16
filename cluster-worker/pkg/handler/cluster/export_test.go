package cluster

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// DrainStatusQueue runs queued status checks until none are left that are
// due now, so tests can assert on the result of CheckStatus right away.
func (h *Handler) DrainStatusQueue(ctx context.Context) {
	for h.statusQueue.Len() > 0 {
		h.processNextStatusCheck(ctx)
	}
}

// StatusRequeues reports how often a cluster's status check was re-queued
// after a failure.
func (h *Handler) StatusRequeues(clusterID uuid.UUID) int {
	return h.statusQueue.NumRequeues(clusterID)
}

// StatusQueueLen reports how many status checks are queued.
func (h *Handler) StatusQueueLen() int {
	return h.statusQueue.Len()
}

// CheckClusterRetryAfter runs one cluster's status check and reports how soon
// it asks to be checked again.
func (h *Handler) CheckClusterRetryAfter(ctx context.Context, clusterID uuid.UUID) (time.Duration, error) {
	return h.checkCluster(ctx, clusterID)
}
