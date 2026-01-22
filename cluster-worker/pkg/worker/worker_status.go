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

func NewStatusWorker(pool *pgxpool.Pool, gardenerClient gardener.Client, logger *slog.Logger, cfg StatusConfig) *StatusWorker {
	return &StatusWorker{
		pool:     pool,
		queries:  db.New(pool),
		gardener: gardenerClient,
		logger:   logger,
		cfg:      cfg,
	}
}

func (p *StatusWorker) Run(ctx context.Context) error {
	ticker := time.NewTicker(p.cfg.PollInterval)
	defer ticker.Stop()

	p.pollBatch(ctx) // Initial poll on startup

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
	p.pollActiveClusters(ctx)
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

		// Look up namespace from Gardener project (by organization ID label)
		projectName := gardener.ProjectName(cluster.OrganizationName)
		namespace, err := p.gardener.EnsureProject(ctx, projectName, cluster.OrganizationID)
		if err != nil {
			p.logger.Error("failed to get project namespace",
				"cluster_id", cluster.ID,
				"error", err)
			continue
		}
		if namespace == "" {
			// Project exists but namespace not ready yet
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

		shootStatus, err := p.gardener.GetShootStatus(ctx, clusterToSync)
		if err != nil {
			p.logger.Error("failed to get shoot status",
				"cluster_id", cluster.ID,
				"error", err)
			continue
		}

		var oldStatus gardener.ShootStatusType
		if cluster.ShootStatus.Valid {
			oldStatus = gardener.ShootStatusType(cluster.ShootStatus.String)
		}

		if err := p.queries.ClusterUpdateShootStatus(ctx, db.ClusterUpdateShootStatusParams{
			ClusterID: cluster.ID,
			Status:    pgtype.Text{String: string(shootStatus.Status), Valid: true},
			Message:   pgtype.Text{String: shootStatus.Message, Valid: true},
		}); err != nil {
			p.logger.Error("failed to update shoot status",
				"cluster_id", cluster.ID,
				"error", err)
			continue
		}

		if shootStatus.Status != oldStatus {
			var eventType db.TenantClusterEventType
			switch shootStatus.Status {
			case gardener.StatusProgressing:
				eventType = db.TenantClusterEventTypeStatusProgressing
			case gardener.StatusReady:
				eventType = db.TenantClusterEventTypeStatusReady
			case gardener.StatusError:
				eventType = db.TenantClusterEventTypeStatusError
			case gardener.StatusPending, gardener.StatusDeleting:
				// No event for these transient states
			case gardener.StatusDeleted:
				// Handled in pollDeletedClusters
			}

			if eventType != "" {
				if _, err := p.queries.ClusterCreateStatusEvent(ctx, db.ClusterCreateStatusEventParams{
					ClusterID: cluster.ID,
					EventType: eventType,
					Message:   pgtype.Text{String: shootStatus.Message, Valid: true},
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
			"status", shootStatus.Status)

		if shootStatus.Status == gardener.StatusError {
			p.logger.Error("ALERT: shoot reconciliation failed",
				"cluster_id", cluster.ID,
				"name", cluster.Name,
				"message", shootStatus.Message)
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
		var deleted *time.Time
		if cluster.Deleted.Valid {
			deleted = &cluster.Deleted.Time
		}

		projectName := gardener.ProjectName(cluster.OrganizationName)
		namespace, err := p.gardener.EnsureProject(ctx, projectName, cluster.OrganizationID)
		if err != nil {
			p.logger.Error("failed to get project namespace",
				"cluster_id", cluster.ID,
				"error", err)
			continue
		}
		if namespace == "" {
			// Project exists but namespace not ready yet
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

		shootStatus, err := p.gardener.GetShootStatus(ctx, clusterToSync)
		if err != nil {
			p.logger.Error("failed to check deleted shoot status",
				"cluster_id", cluster.ID,
				"error", err)
			continue
		}

		// If status is "pending" with "not found", the Shoot is confirmed deleted
		if shootStatus.Status == gardener.StatusPending && shootStatus.Message == gardener.MsgShootNotFound {
			if err := p.queries.ClusterUpdateShootStatus(ctx, db.ClusterUpdateShootStatusParams{
				ClusterID: cluster.ID,
				Status:    pgtype.Text{String: string(gardener.StatusDeleted), Valid: true},
				Message:   pgtype.Text{String: "Shoot confirmed deleted", Valid: true},
			}); err != nil {
				p.logger.Error("failed to update deleted status",
					"cluster_id", cluster.ID,
					"error", err)
				continue
			}

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
				Status:    pgtype.Text{String: string(gardener.StatusDeleting), Valid: true},
				Message:   pgtype.Text{String: shootStatus.Message, Valid: true},
			}); err != nil {
				p.logger.Error("failed to update deleting status",
					"cluster_id", cluster.ID,
					"error", err)
			}
			p.logger.Debug("shoot still being deleted",
				"cluster_id", cluster.ID,
				"status", shootStatus.Status)
		}
	}
}
