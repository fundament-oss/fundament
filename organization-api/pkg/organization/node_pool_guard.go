package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
)

// A live cluster always keeps a node pool that can run nodes. cluster-worker
// builds no Shoot without one, and Gardener refuses a Shoot whose every worker
// pool has a maximum of 0, so a cluster without such a pool would leave its
// Shoot out of step with its pools.
const (
	msgLastNodePool   = "a cluster needs at least one node pool; add another node pool before removing this one"
	msgNoActivePool   = "a cluster needs at least one node pool with a maximum above 0"
	msgFirstPoolEmpty = "a cluster's first node pool needs a maximum above 0"
)

// otherActivePools counts the cluster's live pools with a maximum above 0,
// other than except (if valid). The caller holds the cluster row lock, so the
// count cannot change before its write commits.
func otherActivePools(ctx context.Context, qtx *db.Queries, clusterID uuid.UUID, except pgtype.UUID) (int64, error) {
	n, err := qtx.NodePoolCountActiveByClusterID(ctx, db.NodePoolCountActiveByClusterIDParams{
		ClusterID:        clusterID,
		ExceptNodePoolID: except,
	})
	if err != nil {
		return 0, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to count node pools: %w", err))
	}
	return n, nil
}

// failedPrecondition is the refusal for a change that would leave a live
// cluster without a pool that can run nodes.
func failedPrecondition(message string) error {
	return connect.NewError(connect.CodeFailedPrecondition, errors.New(message))
}
