package cluster

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/common/kubename"
)

// Gardener's report is shoot-wide, and the shoot's own Prometheus only comes
// once every pool has its machines: during a create that hangs on one pool,
// neither says which pools have theirs. The shoot's API server does, by the
// worker.gardener.cloud/pool label on its nodes. These counts decide a pool's
// status (classifyNodePool) and are stored for the console meanwhile.

// PoolNodeReader reads a shoot's nodes per worker pool.
type PoolNodeReader interface {
	ListNodePools(ctx context.Context, clusterID uuid.UUID) (map[string][]shoot.PoolNode, error)
}

// SetPoolNodeReader lets the status check read each pool's nodes from the
// shoot. Without one, pools are classified from Gardener's report alone.
func (h *Handler) SetPoolNodeReader(r PoolNodeReader) {
	h.poolNodes = r
}

// nodeCountMaxAge is how long a stored count that did not change stays
// before a read records it again, so the console can tell a fresh count from
// one that stopped being read.
const nodeCountMaxAge = time.Minute

// poolNodeRecheck is how soon the status is checked again while nodes are
// read: a pool's nodes come up without Gardener writing the Shoot, so no
// Shoot event would bring the check back.
const poolNodeRecheck = time.Minute

// poolNodeCounts is one pool's nodes as the shoot's API server lists them.
type poolNodeCounts struct {
	// all is every node with the pool's label: what the console shows.
	all shoot.PoolNodes
	// own is the nodes created since the pool was: what decides its status.
	own shoot.PoolNodes
}

// readsPoolNodes reports whether this check reads the pools' nodes: once the
// API server answers, while a pool still waits for its machines or the
// shoot's create runs. The create deploys the shoot's Prometheus last, so the
// stored counts are what the console has until it does.
func readsPoolNodes(stored []storedNodePool, observed *gardener.ShootStatus, pushed bool) bool {
	if !observed.APIServerAvailable {
		return false
	}
	if creatingShoot(observed) {
		return true
	}
	for _, pool := range stored {
		if awaitsMachines(pool.Status, pushed) {
			return true
		}
	}
	return false
}

// poolNodesFor returns each pool's node counts by pool name, or nil when
// they are not read (see readsPoolNodes) or the read fails; the shoot-wide
// rules then decide alone.
func (h *Handler) poolNodesFor(ctx context.Context, clusterID uuid.UUID, stored []storedNodePool) map[string]poolNodeCounts {
	byWorker, err := h.poolNodes.ListNodePools(ctx, clusterID)
	if err != nil {
		if shoot.IsUnreachable(err) {
			h.logger.Debug("shoot API server not reachable for node pool counts", "cluster_id", clusterID, "error", err)
		} else {
			h.logger.Warn("failed to read node pool counts", "cluster_id", clusterID, "error", err)
		}
		return nil
	}

	counts := make(map[string]poolNodeCounts, len(stored))
	for _, pool := range stored {
		nodes := byWorker[kubename.GenerateWorkerName(pool.Name)]
		counts[pool.Name] = poolNodeCounts{
			all: shoot.CountPoolNodes(nodes, time.Time{}),
			own: shoot.CountPoolNodes(nodes, pool.Created),
		}
	}
	return counts
}

// ownNodeCounts is the counts that decide each pool's status.
func ownNodeCounts(counts map[string]poolNodeCounts) map[string]shoot.PoolNodes {
	if counts == nil {
		return nil
	}
	own := make(map[string]shoot.PoolNodes, len(counts))
	for name, c := range counts {
		own[name] = c.own
	}
	return own
}

// nodeCountChanged reports whether counts differ from what is stored for the
// pool, or what is stored is older than nodeCountMaxAge.
func nodeCountChanged(pool storedNodePool, counts shoot.PoolNodes, now time.Time) bool {
	return pool.Nodes == nil || *pool.Nodes != counts || now.Sub(pool.NodesRead) >= nodeCountMaxAge
}

// writeNodePoolCount stores one pool's counts on their own, for a pool whose
// status did not change.
func (h *Handler) writeNodePoolCount(ctx context.Context, poolID uuid.UUID, counts shoot.PoolNodes) error {
	_, err := h.queries.NodePoolUpdateNodeCount(ctx, nodeCountParams(poolID, counts))
	return err //nolint:wrapcheck // the caller logs it with the pool
}

func nodeCountParams(poolID uuid.UUID, counts shoot.PoolNodes) db.NodePoolUpdateNodeCountParams {
	return db.NodePoolUpdateNodeCountParams{
		NodePoolID:     poolID,
		NodeCount:      pgtype.Int4{Int32: counts.Nodes, Valid: true},
		NodeCountReady: pgtype.Int4{Int32: counts.Ready, Valid: true},
	}
}
