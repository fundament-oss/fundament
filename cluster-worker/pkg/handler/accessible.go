package handler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
)

// shootUnreachableFirstRetry is how soon a sync deferred by
// NewShootUnreachableError is first retried: the external name of a new
// shoot's API server usually resolves within seconds.
const shootUnreachableFirstRetry = 5 * time.Second

// NewShootUnreachableError defers a sync whose shoot does not answer from here
// yet. Gardener reports the API server available once it answers from the
// seed; its external name can take a little longer. Deferring costs no retry
// and records no failure.
func NewShootUnreachableError(err error) *PreconditionError {
	return NewPreconditionErrorWithFirstRetry("shoot API server not reachable yet: "+err.Error(), shootUnreachableFirstRetry)
}

// ReconcileAccessible runs reconcile for every cluster, isolating per-cluster
// failures. A cluster that is not ready yet and whose shoot does not answer
// from here is skipped with a warning instead of failing the tick: three
// failed ticks stop the reconcile worker. It returns nil once ctx is done.
func ReconcileAccessible(
	ctx context.Context,
	logger *slog.Logger,
	clusters []db.ClusterListAccessibleRow,
	reconcile func(ctx context.Context, clusterID uuid.UUID) error,
) error {
	var errs []error
	for _, cluster := range clusters {
		if ctx.Err() != nil {
			return nil
		}
		err := reconcile(ctx, cluster.ID)
		if err == nil {
			continue
		}
		if !cluster.Ready && shoot.IsUnreachable(err) {
			logger.WarnContext(ctx, "cluster API server not reachable yet, skipping", "cluster_id", cluster.ID, "error", err)
			continue
		}
		logger.ErrorContext(ctx, "failed to reconcile cluster", "cluster_id", cluster.ID, "error", err)
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
