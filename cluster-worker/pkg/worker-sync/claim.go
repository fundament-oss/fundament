package worker_sync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
)

// claimedCluster holds a claimed cluster's info.
type claimedCluster struct {
	ID                uuid.UUID
	OrganizationID    uuid.UUID
	OrganizationName  string
	Name              string
	Region            string
	KubernetesVersion string
	Deleted           *time.Time
	SyncAttempts      int32
}

// claimCluster atomically claims one unsynced cluster using visibility timeout pattern.
// The claim is held for 10 minutes, after which the cluster becomes reclaimable by other workers.
// The SQL query prioritizes active clusters (create/update) over deleted clusters (delete).
func (w *SyncWorker) claimCluster(ctx context.Context) (*claimedCluster, error) {
	row, err := w.queries.ClusterClaimForSync(ctx, db.ClusterClaimForSyncParams{
		MaxAttempts: w.cfg.MaxAttempts,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // No work available
		}
		return nil, fmt.Errorf("claim cluster: %w", err)
	}

	var deleted *time.Time
	if row.Deleted.Valid {
		deleted = &row.Deleted.Time
	}

	return &claimedCluster{
		ID:                row.ID,
		OrganizationID:    row.OrganizationID,
		OrganizationName:  row.OrganizationName,
		Name:              row.Name,
		Region:            row.Region,
		KubernetesVersion: row.KubernetesVersion,
		Deleted:           deleted,
		SyncAttempts:      row.SyncAttempts,
	}, nil
}
