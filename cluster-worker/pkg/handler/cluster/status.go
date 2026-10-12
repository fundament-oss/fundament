package cluster

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/common/dbconst"
)

// msgShootConfirmedDeleted is the message a soft-deleted cluster gets once its
// Shoot is gone from Gardener.
const msgShootConfirmedDeleted = "Shoot confirmed deleted"

// ErrClusterNotSynced is returned by CheckCluster for a cluster whose first
// sync to Gardener has not completed. A Shoot event can arrive just before the
// outbox row that created the Shoot is marked completed; the check is worth
// repeating shortly.
var ErrClusterNotSynced = errors.New("cluster not synced to Gardener yet")

// CheckStatus is the status sweep: it queues a status check for every cluster
// that has reached Gardener and is not confirmed deleted. Watch events cover
// changes to Shoots that exist; the sweep covers what produces no event, such
// as a Shoot deleted while cluster-worker was not running, or a cluster that
// never got a Shoot.
func (h *Handler) CheckStatus(ctx context.Context) error {
	ids, err := h.queries.ClusterListForStatusSweep(ctx)
	if err != nil {
		h.logger.Error("failed to list clusters for the status sweep", "error", err)
		return fmt.Errorf("list clusters for status sweep: %w", err)
	}
	for _, id := range ids {
		h.EnqueueStatusCheck(id)
	}
	h.logger.Debug("status sweep queued clusters", "count", len(ids))
	return nil
}

// CheckCluster reads one cluster's Shoot status from Gardener and records it.
// It is the single status step: every check, whatever triggered it, ends here.
// A cluster that no longer exists or is confirmed deleted is left alone; one
// that has not reached Gardener yet returns ErrClusterNotSynced.
func (h *Handler) CheckCluster(ctx context.Context, id uuid.UUID) error {
	_, err := h.checkCluster(ctx, id)
	return err
}

// checkCluster is CheckCluster that also reports when the cluster should be
// checked again to record a progress message held back by the throttle.
func (h *Handler) checkCluster(ctx context.Context, id uuid.UUID) (retryAfter time.Duration, err error) {
	cluster, err := h.queries.ClusterGetForStatusCheck(ctx, db.ClusterGetForStatusCheckParams{ClusterID: id})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("load cluster %s: %w", id, err)
	}
	if !cluster.Synced {
		return 0, ErrClusterNotSynced
	}

	// Status lookups find the Shoot by cluster-ID label across all namespaces, so
	// they need no project namespace (and must not create a Project as a side effect).
	clusterToSync := clusterToSyncBase(cluster.ID, cluster.Name, cluster.OrganizationName, cluster.OrganizationID, "", cluster.Region, cluster.KubernetesVersion, cluster.CloudProfile, cluster.CloudProfileRegion)

	if cluster.Deleted.Valid {
		if gardener.ShootStatusType(cluster.ShootStatus.String) == gardener.StatusDeleted {
			return 0, nil
		}
		deleted := cluster.Deleted.Time
		clusterToSync.Deleted = &deleted
		return 0, h.checkDeletedCluster(ctx, &cluster, clusterToSync)
	}
	return h.checkActiveCluster(ctx, &cluster, clusterToSync)
}

// checkActiveCluster applies the status rules to an active cluster.
func (h *Handler) checkActiveCluster(ctx context.Context, cluster *db.ClusterGetForStatusCheckRow, clusterToSync *gardener.ClusterToSync) (time.Duration, error) {
	shootStatus, err := h.statusChecker.GetShootStatus(ctx, clusterToSync)
	if err != nil {
		return 0, fmt.Errorf("get shoot status: %w", err)
	}

	stored := storedShootState{
		Status:   gardener.ShootStatusType(cluster.ShootStatus.String),
		Message:  cluster.ShootStatusMessage.String,
		Health:   dbconst.ClusterShootHealth(cluster.ShootHealth.String),
		Updating: cluster.ShootUpdating,
	}
	if shootStatus.Status == gardener.StatusError && shootStatus.Retrying {
		lastWarning, err := h.queries.ClusterGetLastWarningMessage(ctx, db.ClusterGetLastWarningMessageParams{ClusterID: cluster.ID})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("load last warning: %w", err)
		}
		stored.LastWarning = lastWarning.String
	}
	update := nextShootStatus(stored, shootStatus)
	var retryAfter time.Duration
	if update.Status == gardener.StatusProgressing || update.Updating {
		last, err := h.lastProgress(ctx, cluster.ID)
		if err != nil {
			return 0, err
		}
		retryAfter = addProgressEvent(&update, last, time.Now(), h.cfg.StatusProgressEventInterval)
	}
	if update.unchanged(stored) {
		// Gardener writes a Shoot after every reconcile task; most of those
		// leave what fundament records as it is.
		return retryAfter, nil
	}

	written, err := h.writeShootStatus(ctx, cluster, &update)
	if err != nil {
		return 0, err
	}
	if !written {
		// The row changed after it was read (an update was marked in
		// progress). The check runs again shortly and decides from the new
		// state: the Shoot may already be in its final state, so no further
		// event can be counted on to bring it back.
		h.logger.Debug("shoot status changed during check, checking again", "cluster_id", cluster.ID)
		return statusRetryBaseDelay, nil
	}

	for _, event := range update.Events {
		switch event.Type {
		case dbconst.ClusterEventEventType_StatusUnhealthy:
			h.logger.Warn("ready shoot became unhealthy", "cluster_id", cluster.ID, "name", cluster.Name, "message", event.Message)
		case dbconst.ClusterEventEventType_StatusLost:
			h.logger.Warn("shoot disappeared from Gardener", "cluster_id", cluster.ID, "name", cluster.Name, "previous_status", stored.Status)
		case dbconst.ClusterEventEventType_StatusWarning:
			h.logger.Warn("gardener is retrying a failed shoot operation", "cluster_id", cluster.ID, "name", cluster.Name, "message", event.Message)
		default:
		}
	}

	h.logger.Debug("updated shoot status",
		"cluster_id", cluster.ID,
		"name", cluster.Name,
		"status", update.Status,
		"health", update.Health)

	if update.Status == gardener.StatusError {
		h.logger.Error("ALERT: shoot reconciliation failed",
			"cluster_id", cluster.ID,
			"name", cluster.Name,
			"message", shootStatus.Message)
	}
	return retryAfter, nil
}

// lastProgress loads the cluster's latest status_progressing event.
func (h *Handler) lastProgress(ctx context.Context, clusterID uuid.UUID) (progressRecord, error) {
	row, err := h.queries.ClusterGetLastProgressEvent(ctx, db.ClusterGetLastProgressEventParams{ClusterID: clusterID})
	if errors.Is(err, pgx.ErrNoRows) {
		return progressRecord{}, nil
	}
	if err != nil {
		return progressRecord{}, fmt.Errorf("load last progress event: %w", err)
	}
	return progressRecord{At: row.Created.Time, Message: row.Message.String}, nil
}

// checkDeletedCluster confirms that a soft-deleted cluster's Shoot is gone.
func (h *Handler) checkDeletedCluster(ctx context.Context, cluster *db.ClusterGetForStatusCheckRow, clusterToSync *gardener.ClusterToSync) error {
	shootStatus, err := h.statusChecker.GetShootStatus(ctx, clusterToSync)
	if err != nil {
		return fmt.Errorf("get deleted shoot status: %w", err)
	}

	if shootStatus.Status != gardener.StatusPending || shootStatus.Message != gardener.MsgShootNotFound {
		deleting := shootStatusUpdate{Status: gardener.StatusDeleting, Message: shootStatus.Message}
		if deleting.unchanged(storedShootState{Status: gardener.ShootStatusType(cluster.ShootStatus.String), Message: cluster.ShootStatusMessage.String}) {
			return nil
		}
		if _, err := h.writeShootStatus(ctx, cluster, &deleting); err != nil {
			return err
		}
		h.logger.Debug("shoot still being deleted", "cluster_id", cluster.ID, "status", shootStatus.Status)
		return nil
	}

	written, err := h.writeShootStatus(ctx, cluster, &shootStatusUpdate{
		Status:  gardener.StatusDeleted,
		Message: msgShootConfirmedDeleted,
		Events:  []statusEvent{{Type: dbconst.ClusterEventEventType_StatusDeleted, Message: msgShootConfirmedDeleted}},
	})
	if err != nil {
		return err
	}
	if written {
		h.logger.Info("confirmed shoot deletion", "cluster_id", cluster.ID, "name", cluster.Name)
	}
	return nil
}

// writeShootStatus records a status update with its events and, on a
// transition into ready, the ready outbox row, all in one transaction. The
// update only applies while shoot_status_updated still holds the value read
// with the cluster, so of two checks racing on one row exactly one writes and
// records the follow-up rows. It reports whether the update applied.
func (h *Handler) writeShootStatus(ctx context.Context, cluster *db.ClusterGetForStatusCheckRow, update *shootStatusUpdate) (bool, error) {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin status transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	qtx := h.queries.WithTx(tx)

	updated, err := qtx.ClusterUpdateShootStatus(ctx, db.ClusterUpdateShootStatusParams{
		ClusterID: cluster.ID,
		Status:    pgtype.Text{String: string(update.Status), Valid: true},
		Message:   pgtype.Text{String: update.Message, Valid: true},
		Health:    pgtype.Text{String: string(update.Health), Valid: update.Health != ""},
		Updating:  update.Updating,
		CheckedAt: cluster.ShootStatusUpdated,
	})
	if err != nil {
		return false, fmt.Errorf("update shoot status: %w", err)
	}
	if updated == 0 {
		return false, nil
	}

	for _, event := range update.Events {
		if _, err := qtx.ClusterCreateStatusEvent(ctx, db.ClusterCreateStatusEventParams{
			ClusterID: cluster.ID,
			EventType: string(event.Type),
			Message:   pgtype.Text{String: event.Message, Valid: true},
		}); err != nil {
			return false, fmt.Errorf("create %s event: %w", event.Type, err)
		}
	}

	// On transition to ready, insert a ready outbox row. Handlers that
	// react to cluster-ready (usersync, namespace-sync) subscribe to this
	// event via the registry — the status handler stays agnostic of them.
	if update.InsertReady {
		if err := qtx.OutboxInsertReady(ctx, db.OutboxInsertReadyParams{
			ClusterID: pgtype.UUID{Bytes: cluster.ID, Valid: true},
		}); err != nil {
			return false, fmt.Errorf("insert ready outbox row: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit status transaction: %w", err)
	}
	return true, nil
}
