package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
)

// claimedCluster holds a claimed cluster's info.
type claimedCluster struct {
	ID                uuid.UUID
	Name              string
	OrganizationName  string
	Region            string
	KubernetesVersion string
	Deleted           *time.Time
	SyncAttempts      int32
}

// claimCluster atomically claims one unsynced cluster using visibility timeout pattern.
// The claim is held for 10 minutes, after which the cluster becomes reclaimable by other workers.
// Prioritizes active clusters (create/update) over deleted clusters (delete).
func (w *SyncWorker) claimCluster(ctx context.Context) (*claimedCluster, error) {
	// Try to claim an active cluster first (create/update flow)
	cluster, err := w.claimActiveCluster(ctx)
	if err != nil {
		return nil, err
	}
	if cluster != nil {
		return cluster, nil
	}

	// No active clusters, try deleted clusters (delete flow)
	return w.claimDeletedCluster(ctx)
}

// claimActiveCluster claims an active (non-deleted) cluster for create/update sync.
func (w *SyncWorker) claimActiveCluster(ctx context.Context) (*claimedCluster, error) {
	workerID := pgtype.Text{String: w.workerID, Valid: true}

	row, err := w.queries.ClusterClaimForSync(ctx, db.ClusterClaimForSyncParams{
		WorkerID:    workerID,
		MaxAttempts: w.cfg.MaxAttempts,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // No work available
		}
		return nil, fmt.Errorf("claim active cluster: %w", err)
	}

	return &claimedCluster{
		ID:                row.ID,
		Name:              row.Name,
		OrganizationName:  row.OrganizationName,
		Region:            row.Region,
		KubernetesVersion: row.KubernetesVersion,
		Deleted:           nil, // Active clusters are never deleted
		SyncAttempts:      row.SyncAttempts,
	}, nil
}

// claimDeletedCluster claims a soft-deleted cluster for deletion sync.
func (w *SyncWorker) claimDeletedCluster(ctx context.Context) (*claimedCluster, error) {
	workerID := pgtype.Text{String: w.workerID, Valid: true}

	row, err := w.queries.ClusterClaimDeletedForSync(ctx, db.ClusterClaimDeletedForSyncParams{
		WorkerID:    workerID,
		MaxAttempts: w.cfg.MaxAttempts,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // No work available
		}
		return nil, fmt.Errorf("claim deleted cluster: %w", err)
	}

	var deleted *time.Time
	if row.Deleted.Valid {
		deleted = &row.Deleted.Time
	}

	return &claimedCluster{
		ID:                row.ID,
		Name:              row.Name,
		OrganizationName:  row.OrganizationName,
		Region:            row.Region,
		KubernetesVersion: row.KubernetesVersion,
		Deleted:           deleted,
		SyncAttempts:      row.SyncAttempts,
	}, nil
}
