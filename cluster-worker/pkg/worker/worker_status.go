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
	PollInterval time.Duration // How often to poll (e.g., 30s)
	BatchSize    int32         // Max clusters to check per poll cycle (e.g., 50)
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
	clusters, err := p.queries.ListClustersNeedingStatusCheck(ctx, p.cfg.BatchSize)
	if err != nil {
		p.logger.Error("failed to list clusters for status check", "error", err)
		return
	}

	for _, cluster := range clusters {
		clusterToSync := &gardener.ClusterToSync{
			ID:                cluster.ID,
			Name:              cluster.Name,
			OrganizationName:  cluster.OrganizationName,
			Region:            cluster.Region,
			KubernetesVersion: cluster.KubernetesVersion,
		}

		status, message, err := p.gardener.GetShootStatus(ctx, clusterToSync)
		if err != nil {
			p.logger.Error("failed to get shoot status",
				"cluster_id", cluster.ID,
				"error", err)
			continue
		}

		if err := p.queries.UpdateShootStatus(ctx, db.UpdateShootStatusParams{
			ClusterID:          cluster.ID,
			ShootStatus:        pgtype.Text{String: status, Valid: true},
			ShootStatusMessage: pgtype.Text{String: message, Valid: true},
		}); err != nil {
			p.logger.Error("failed to update shoot status",
				"cluster_id", cluster.ID,
				"error", err)
			continue
		}

		p.logger.Info("updated shoot status",
			"cluster_id", cluster.ID,
			"name", cluster.Name,
			"status", status)

		if status == "error" {
			p.logger.Error("ALERT: shoot reconciliation failed",
				"cluster_id", cluster.ID,
				"name", cluster.Name,
				"message", message)
		}
	}
}

func (p *StatusWorker) pollDeletedClusters(ctx context.Context) {
	clusters, err := p.queries.ListDeletedClustersNeedingVerification(ctx, p.cfg.BatchSize)
	if err != nil {
		p.logger.Error("failed to list deleted clusters for verification", "error", err)
		return
	}

	for _, cluster := range clusters {
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
		if status == "pending" && message == "Shoot not found in Gardener" {
			if err := p.queries.UpdateShootStatus(ctx, db.UpdateShootStatusParams{
				ClusterID:          cluster.ID,
				ShootStatus:        pgtype.Text{String: "deleted", Valid: true},
				ShootStatusMessage: pgtype.Text{String: "Shoot confirmed deleted", Valid: true},
			}); err != nil {
				p.logger.Error("failed to update deleted status",
					"cluster_id", cluster.ID,
					"error", err)
				continue
			}
			p.logger.Info("confirmed shoot deletion",
				"cluster_id", cluster.ID,
				"name", cluster.Name)
		} else {
			// Shoot still exists or is being deleted
			if err := p.queries.UpdateShootStatus(ctx, db.UpdateShootStatusParams{
				ClusterID:          cluster.ID,
				ShootStatus:        pgtype.Text{String: "deleting", Valid: true},
				ShootStatusMessage: pgtype.Text{String: message, Valid: true},
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
