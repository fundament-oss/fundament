package cluster

import (
	"context"

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
