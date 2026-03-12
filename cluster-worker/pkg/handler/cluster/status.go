package cluster

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/common/dbconst"
)

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

// pollActiveClusters checks Gardener status for active (non-deleted) clusters.
func (h *Handler) pollActiveClusters(ctx context.Context) error {
	clusters, err := h.queries.ClusterListNeedingStatusCheck(ctx, db.ClusterListNeedingStatusCheckParams{
		LimitCount: h.cfg.StatusBatchSize,
	})
	if err != nil {
		h.logger.Error("failed to list clusters for status check", "error", err)
		return fmt.Errorf("list clusters for status check: %w", err)
	}

	for i := range clusters {
		if ctx.Err() != nil {
			return nil //nolint:nilerr // graceful shutdown
		}
		cluster := &clusters[i]

		projectName := gardener.ProjectName(cluster.OrganizationName)
		namespace, err := h.gardener.EnsureProject(ctx, projectName, cluster.OrganizationID)
		if err != nil {
			h.logger.Error("failed to get project namespace",
				"cluster_id", cluster.ID,
				"error", err)
			continue
		}
		if namespace == "" {
			continue
		}

		clusterToSync := &gardener.ClusterToSync{
			ID:                cluster.ID,
			Name:              cluster.Name,
			OrganizationID:    cluster.OrganizationID,
			OrganizationName:  cluster.OrganizationName,
			Namespace:         namespace,
			Region:            cluster.Region,
			KubernetesVersion: cluster.KubernetesVersion,
		}

		shootStatus, err := h.gardener.GetShootStatus(ctx, clusterToSync)
		if err != nil {
			h.logger.Error("failed to get shoot status",
				"cluster_id", cluster.ID,
				"error", err)
			continue
		}

		var oldStatus gardener.ShootStatusType
		if cluster.ShootStatus.Valid {
			oldStatus = gardener.ShootStatusType(cluster.ShootStatus.String)
		}

		if err := h.queries.ClusterUpdateShootStatus(ctx, db.ClusterUpdateShootStatusParams{
			ClusterID: cluster.ID,
			Status:    pgtype.Text{String: string(shootStatus.Status), Valid: true},
			Message:   pgtype.Text{String: shootStatus.Message, Valid: true},
		}); err != nil {
			h.logger.Error("failed to update shoot status",
				"cluster_id", cluster.ID,
				"error", err)
			continue
		}

		if shootStatus.Status != oldStatus {
			var eventType dbconst.ClusterEventEventType
			switch shootStatus.Status {
			case gardener.StatusProgressing:
				eventType = dbconst.ClusterEventEventType_StatusProgressing
			case gardener.StatusReady:
				eventType = dbconst.ClusterEventEventType_StatusReady
			case gardener.StatusError:
				eventType = dbconst.ClusterEventEventType_StatusError
			case gardener.StatusPending, gardener.StatusDeleting:
				// No event for these transient states
			case gardener.StatusDeleted:
				// Handled in pollDeletedClusters
			default:
				panic(fmt.Sprintf("unhandled shoot status: %s", shootStatus.Status))
			}

			if eventType != "" {
				if _, err := h.queries.ClusterCreateStatusEvent(ctx, db.ClusterCreateStatusEventParams{
					ClusterID: cluster.ID,
					EventType: string(eventType),
					Message:   pgtype.Text{String: shootStatus.Message, Valid: true},
				}); err != nil {
					h.logger.Warn("failed to create status event",
						"cluster_id", cluster.ID,
						"event_type", eventType,
						"error", err)
				}
			}
		}

		h.logger.Info("updated shoot status",
			"cluster_id", cluster.ID,
			"name", cluster.Name,
			"status", shootStatus.Status)

		if shootStatus.Status == gardener.StatusError {
			h.logger.Error("ALERT: shoot reconciliation failed",
				"cluster_id", cluster.ID,
				"name", cluster.Name,
				"message", shootStatus.Message)
		}
	}
	return nil
}

// pollDeletedClusters verifies that soft-deleted clusters have actually been removed from Gardener.
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
		cluster := &clusters[i]
		var deleted *time.Time
		if cluster.Deleted.Valid {
			deleted = &cluster.Deleted.Time
		}

		projectName := gardener.ProjectName(cluster.OrganizationName)
		namespace, err := h.gardener.EnsureProject(ctx, projectName, cluster.OrganizationID)
		if err != nil {
			h.logger.Error("failed to get project namespace",
				"cluster_id", cluster.ID,
				"error", err)
			continue
		}
		if namespace == "" {
			continue
		}

		clusterToSync := &gardener.ClusterToSync{
			ID:                cluster.ID,
			Name:              cluster.Name,
			OrganizationID:    cluster.OrganizationID,
			OrganizationName:  cluster.OrganizationName,
			Namespace:         namespace,
			Region:            cluster.Region,
			KubernetesVersion: cluster.KubernetesVersion,
			Deleted:           deleted,
		}

		shootStatus, err := h.gardener.GetShootStatus(ctx, clusterToSync)
		if err != nil {
			h.logger.Error("failed to check deleted shoot status",
				"cluster_id", cluster.ID,
				"error", err)
			continue
		}

		if shootStatus.Status == gardener.StatusPending && shootStatus.Message == gardener.MsgShootNotFound {
			if err := h.queries.ClusterUpdateShootStatus(ctx, db.ClusterUpdateShootStatusParams{
				ClusterID: cluster.ID,
				Status:    pgtype.Text{String: string(gardener.StatusDeleted), Valid: true},
				Message:   pgtype.Text{String: "Shoot confirmed deleted", Valid: true},
			}); err != nil {
				h.logger.Error("failed to update deleted status",
					"cluster_id", cluster.ID,
					"error", err)
				continue
			}

			if _, err := h.queries.ClusterCreateStatusEvent(ctx, db.ClusterCreateStatusEventParams{
				ClusterID: cluster.ID,
				EventType: string(dbconst.ClusterEventEventType_StatusDeleted),
				Message:   pgtype.Text{String: "Shoot confirmed deleted", Valid: true},
			}); err != nil {
				h.logger.Warn("failed to create status_deleted event",
					"cluster_id", cluster.ID,
					"error", err)
			}

			h.logger.Info("confirmed shoot deletion",
				"cluster_id", cluster.ID,
				"name", cluster.Name)
		} else {
			if err := h.queries.ClusterUpdateShootStatus(ctx, db.ClusterUpdateShootStatusParams{
				ClusterID: cluster.ID,
				Status:    pgtype.Text{String: string(gardener.StatusDeleting), Valid: true},
				Message:   pgtype.Text{String: shootStatus.Message, Valid: true},
			}); err != nil {
				h.logger.Error("failed to update deleting status",
					"cluster_id", cluster.ID,
					"error", err)
			}
			h.logger.Debug("shoot still being deleted",
				"cluster_id", cluster.ID,
				"status", shootStatus.Status)
		}
	}
	return nil
}
