package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/gardener"
)

// StatusWorker monitors Shoot reconciliation status in Gardener.
// It runs separately from the main worker to avoid blocking sync operations.
type StatusWorker struct {
	pool     *pgxpool.Pool
	queries  *db.Queries
	gardener gardener.Client
	logger   *slog.Logger
	cfg      StatusConfig
}

// StatusConfig holds configuration for the status poller.
type StatusConfig struct {
	PollInterval time.Duration `env:"POLL_INTERVAL" envDefault:"30s"` // How often to poll
	BatchSize    int32         `env:"BATCH_SIZE" envDefault:"50"`     // Max clusters to check per poll cycle
}

// NewStatusWorker creates a new StatusWorker.
func NewStatusWorker(pool *pgxpool.Pool, gardenerClient gardener.Client, logger *slog.Logger, cfg StatusConfig) *StatusWorker {
	return &StatusWorker{
		pool:     pool,
		queries:  db.New(pool),
		gardener: gardenerClient,
		logger:   logger,
		cfg:      cfg,
	}
}

// Run starts the status polling loop.
func (p *StatusWorker) Run(ctx context.Context) error {
	ticker := time.NewTicker(p.cfg.PollInterval)
	defer ticker.Stop()

	// Do an initial poll immediately on startup
	p.pollBatch(ctx)

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("status poller stopped: %w", ctx.Err())
		case <-ticker.C:
			p.pollBatch(ctx)
		}
	}
}

func (p *StatusWorker) pollBatch(ctx context.Context) {
	// 1. Check active clusters for readiness
	p.pollActiveClusters(ctx)

	// 2. Verify deleted clusters are actually gone from Gardener
	p.pollDeletedClusters(ctx)
}

func (p *StatusWorker) pollActiveClusters(ctx context.Context) {
	clusters, err := p.queries.ClusterListNeedingStatusCheck(ctx, db.ClusterListNeedingStatusCheckParams{
		LimitCount: p.cfg.BatchSize,
	})
	if err != nil {
		p.logger.Error("failed to list clusters for status check", "error", err)
		return
	}

	for i := range clusters {
		cluster := &clusters[i]
		clusterToSync := &gardener.ClusterToSync{
			ID:                cluster.ID,
			Name:              cluster.Name,
			OrganizationName:  cluster.OrganizationName,
			Region:            cluster.Region,
			KubernetesVersion: cluster.KubernetesVersion,
		}

		newStatus, message, err := p.gardener.GetShootStatus(ctx, clusterToSync)
		if err != nil {
			p.logger.Error("failed to get shoot status",
				"cluster_id", cluster.ID,
				"error", err)
			continue
		}

		// Get previous status for event creation
		oldStatus := ""
		if cluster.ShootStatus.Valid {
			oldStatus = cluster.ShootStatus.String
		}

		// Always update shoot status in DB
		if err := p.queries.ClusterUpdateShootStatus(ctx, db.ClusterUpdateShootStatusParams{
			ClusterID: cluster.ID,
			Status:    pgtype.Text{String: newStatus, Valid: true},
			Message:   pgtype.Text{String: message, Valid: true},
		}); err != nil {
			p.logger.Error("failed to update shoot status",
				"cluster_id", cluster.ID,
				"error", err)
			continue
		}

		// Create events only for milestone state changes
		if newStatus != oldStatus {
			var eventType db.TenantClusterEventType
			switch newStatus {
			case gardener.StatusReady:
				eventType = db.TenantClusterEventTypeStatusReady
			case gardener.StatusError:
				eventType = db.TenantClusterEventTypeStatusError
				// Note: status_deleted is handled in pollDeletedClusters
			}

			if eventType != "" {
				if _, err := p.queries.ClusterCreateStatusEvent(ctx, db.ClusterCreateStatusEventParams{
					ClusterID: cluster.ID,
					EventType: eventType,
					Message:   pgtype.Text{String: message, Valid: true},
				}); err != nil {
					p.logger.Warn("failed to create status event",
						"cluster_id", cluster.ID,
						"event_type", eventType,
						"error", err)
				}
			}
		}

		p.logger.Info("updated shoot status",
			"cluster_id", cluster.ID,
			"name", cluster.Name,
			"status", newStatus)

		if newStatus == gardener.StatusError {
			p.logger.Error("ALERT: shoot reconciliation failed",
				"cluster_id", cluster.ID,
				"name", cluster.Name,
				"message", message)
		}
	}
}

func (p *StatusWorker) pollDeletedClusters(ctx context.Context) {
	clusters, err := p.queries.ClusterListDeletedNeedingVerification(ctx, db.ClusterListDeletedNeedingVerificationParams{
		LimitCount: p.cfg.BatchSize,
	})
	if err != nil {
		p.logger.Error("failed to list deleted clusters for verification", "error", err)
		return
	}

	for i := range clusters {
		cluster := &clusters[i]
		// Convert pgtype to time pointer
		var deleted *time.Time
		if cluster.Deleted.Valid {
			deleted = &cluster.Deleted.Time
		}

		clusterToSync := &gardener.ClusterToSync{
			ID:                cluster.ID,
			Name:              cluster.Name,
			OrganizationName:  cluster.OrganizationName,
			Region:            cluster.Region,
			KubernetesVersion: cluster.KubernetesVersion,
			Deleted:           deleted,
		}

		status, message, err := p.gardener.GetShootStatus(ctx, clusterToSync)
		if err != nil {
			p.logger.Error("failed to check deleted shoot status",
				"cluster_id", cluster.ID,
				"error", err)
			continue
		}

		// If status is "pending" with "not found", the Shoot is confirmed deleted
		if status == gardener.StatusPending && message == gardener.MsgShootNotFound {
			if err := p.queries.ClusterUpdateShootStatus(ctx, db.ClusterUpdateShootStatusParams{
				ClusterID: cluster.ID,
				Status:    pgtype.Text{String: gardener.StatusDeleted, Valid: true},
				Message:   pgtype.Text{String: "Shoot confirmed deleted", Valid: true},
			}); err != nil {
				p.logger.Error("failed to update deleted status",
					"cluster_id", cluster.ID,
					"error", err)
				continue
			}

			// Create status_deleted event
			if _, err := p.queries.ClusterCreateStatusEvent(ctx, db.ClusterCreateStatusEventParams{
				ClusterID: cluster.ID,
				EventType: db.TenantClusterEventTypeStatusDeleted,
				Message:   pgtype.Text{String: "Shoot confirmed deleted", Valid: true},
			}); err != nil {
				p.logger.Warn("failed to create status_deleted event",
					"cluster_id", cluster.ID,
					"error", err)
			}

			p.logger.Info("confirmed shoot deletion",
				"cluster_id", cluster.ID,
				"name", cluster.Name)
		} else {
			// Shoot still exists or is being deleted
			if err := p.queries.ClusterUpdateShootStatus(ctx, db.ClusterUpdateShootStatusParams{
				ClusterID: cluster.ID,
				Status:    pgtype.Text{String: gardener.StatusDeleting, Valid: true},
				Message:   pgtype.Text{String: message, Valid: true},
			}); err != nil {
				p.logger.Error("failed to update deleting status",
					"cluster_id", cluster.ID,
					"error", err)
			}
			p.logger.Debug("shoot still being deleted",
				"cluster_id", cluster.ID,
				"status", status)
		}
	}
}
