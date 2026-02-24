package worker_outbox

import "context"

// reconcileAllHandlers delegates reconciliation to each registered ReconcileHandler.
// Each handler owns its own re-enqueue and orphan-detection logic.
func (w *OutboxWorker) reconcileAllHandlers(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	w.logger.Info("starting outbox reconciliation")

	for _, h := range w.registry.ReconcileHandlers() {
		if err := h.Reconcile(ctx); err != nil {
			w.logger.Error("reconcile handler failed", "error", err)
		}
	}

	w.logger.Info("outbox reconciliation complete")
}
