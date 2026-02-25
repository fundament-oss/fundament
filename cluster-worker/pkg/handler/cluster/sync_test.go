package cluster

import "github.com/fundament-oss/fundament/cluster-worker/pkg/handler"

var (
	_ handler.SyncHandler      = (*Handler)(nil)
	_ handler.StatusHandler    = (*Handler)(nil)
	_ handler.ReconcileHandler = (*Handler)(nil)
)
