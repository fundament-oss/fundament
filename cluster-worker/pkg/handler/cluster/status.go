package cluster

import (
	"context"
	"errors"
	"fmt"

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

// CheckStatus polls Gardener for shoot status and updates the database.
func (h *Handler) CheckStatus(ctx context.Context) error {
	var errs []error
	if err := h.pollActiveClusters(ctx); err != nil {
		errs = append(errs, err)
	}
	if err := h.pollDeletedClusters(ctx); err != nil {
		errs = append(errs, err)
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("check status: %w", err)
	}
	return nil
}

// pollActiveClusters checks the active (non-deleted) clusters that are due.
func (h *Handler) pollActiveClusters(ctx context.Context) error {
	clusters, err := h.queries.ClusterListNeedingStatusCheck(ctx, db.ClusterListNeedingStatusCheckParams{
		ReadyInterval: pgtype.Interval{Microseconds: h.cfg.StatusReadyInterval.Microseconds(), Valid: true},
		LimitCount:    h.cfg.StatusBatchSize,
	})
	if err != nil {
		h.logger.Error("failed to list clusters for status check", "error", err)
		return fmt.Errorf("list clusters for status check: %w", err)
	}

	for i := range clusters {
		if ctx.Err() != nil {
			return nil //nolint:nilerr // graceful shutdown
		}
		if err := h.CheckCluster(ctx, clusters[i].ID); err != nil {
			h.logger.Error("failed to check shoot status", "cluster_id", clusters[i].ID, "error", err)
		}
	}
	return nil
}

// pollDeletedClusters checks the soft-deleted clusters whose Shoot is not
// confirmed gone yet.
func (h *Handler) pollDeletedClusters(ctx context.Context) error {
	clusters, err := h.queries.ClusterListDeletedNeedingVerification(ctx, db.ClusterListDeletedNeedingVerificationParams{
		LimitCount: h.cfg.StatusBatchSize,
	})
	if err != nil {
		h.logger.Error("failed to list deleted clusters for verification", "error", err)
		return fmt.Errorf("list deleted clusters for verification: %w", err)
	}

	for i := range clusters {
		if ctx.Err() != nil {
			return nil //nolint:nilerr // graceful shutdown
		}
		if err := h.CheckCluster(ctx, clusters[i].ID); err != nil {
			h.logger.Error("failed to check deleted shoot status", "cluster_id", clusters[i].ID, "error", err)
		}
	}
	return nil
}

// CheckCluster reads one cluster's Shoot status from Gardener and records it.
// It is the single status step: every check, whatever triggered it, ends here.
// A cluster that no longer exists, has not reached Gardener yet, or is
// confirmed deleted is left alone.
func (h *Handler) CheckCluster(ctx context.Context, id uuid.UUID) error {
	cluster, err := h.queries.ClusterGetForStatusCheck(ctx, db.ClusterGetForStatusCheckParams{ClusterID: id})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("load cluster %s: %w", id, err)
	}
	if !cluster.Synced {
		return nil
	}

	// Status lookups find the Shoot by cluster-ID label across all namespaces, so
	// they need no project namespace (and must not create a Project as a side effect).
	clusterToSync := clusterToSyncBase(cluster.ID, cluster.Name, cluster.OrganizationName, cluster.OrganizationID, "", cluster.Region, cluster.KubernetesVersion, cluster.CloudProfile, cluster.CloudProfileRegion)

	if cluster.Deleted.Valid {
		if gardener.ShootStatusType(cluster.ShootStatus.String) == gardener.StatusDeleted {
			return nil
		}
		deleted := cluster.Deleted.Time
		clusterToSync.Deleted = &deleted
		return h.checkDeletedCluster(ctx, &cluster, clusterToSync)
	}
	return h.checkActiveCluster(ctx, &cluster, clusterToSync)
}

// checkActiveCluster applies the status rules to an active cluster.
func (h *Handler) checkActiveCluster(ctx context.Context, cluster *db.ClusterGetForStatusCheckRow, clusterToSync *gardener.ClusterToSync) error {
	shootStatus, err := h.statusChecker.GetShootStatus(ctx, clusterToSync)
	if err != nil {
		return fmt.Errorf("get shoot status: %w", err)
	}

	stored := storedShootState{
		Status:   gardener.ShootStatusType(cluster.ShootStatus.String),
		Message:  cluster.ShootStatusMessage.String,
		Health:   dbconst.ClusterShootHealth(cluster.ShootHealth.String),
		Updating: cluster.ShootUpdating,
	}
	update := nextShootStatus(stored, shootStatus)

	written, err := h.writeShootStatus(ctx, cluster, &update)
	if err != nil {
		return err
	}
	if !written {
		// The row changed after it was read (an update was marked in
		// progress); the next check decides from the new state.
		h.logger.Debug("shoot status changed during check, skipping", "cluster_id", cluster.ID)
		return nil
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
	return nil
}

// checkDeletedCluster confirms that a soft-deleted cluster's Shoot is gone.
func (h *Handler) checkDeletedCluster(ctx context.Context, cluster *db.ClusterGetForStatusCheckRow, clusterToSync *gardener.ClusterToSync) error {
	shootStatus, err := h.statusChecker.GetShootStatus(ctx, clusterToSync)
	if err != nil {
		return fmt.Errorf("get deleted shoot status: %w", err)
	}

	if shootStatus.Status != gardener.StatusPending || shootStatus.Message != gardener.MsgShootNotFound {
		if _, err := h.writeShootStatus(ctx, cluster, &shootStatusUpdate{Status: gardener.StatusDeleting, Message: shootStatus.Message}); err != nil {
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
