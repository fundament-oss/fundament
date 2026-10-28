package organization

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"

	"github.com/fundament-oss/fundament/common/kubename"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func TestNodePoolFromRowStoredStatus(t *testing.T) {
	t.Parallel()
	row := db.NodePoolListByClusterIDRow{Name: "nomach", MachineType: "local-medium", AutoscaleMin: 1, AutoscaleMax: 1}

	row.Status = pgtype.Text{String: "waiting", Valid: true}
	row.StatusMessage = pgtype.Text{String: "no machines", Valid: true}
	pool := nodePoolFromRow(&row, nodePoolRuntime{})
	assert.Equal(t, organizationv1.NodePoolStatus_NODE_POOL_STATUS_WAITING_FOR_MACHINES, pool.GetStatus())
	assert.Equal(t, "no machines", pool.GetStatusMessage())

	row.Status = pgtype.Text{String: "progressing", Valid: true}
	row.StatusMessage = pgtype.Text{}
	assert.Equal(t, organizationv1.NodePoolStatus_NODE_POOL_STATUS_PROVISIONING, nodePoolFromRow(&row, nodePoolRuntime{}).GetStatus())

	row.Status = pgtype.Text{String: "error", Valid: true}
	assert.Equal(t, organizationv1.NodePoolStatus_NODE_POOL_STATUS_FAILED, nodePoolFromRow(&row, nodePoolRuntime{}).GetStatus())

	// Ready (or never classified) defers to the metrics runtime.
	row.Status = pgtype.Text{String: "ready", Valid: true}
	assert.Equal(t, organizationv1.NodePoolStatus_NODE_POOL_STATUS_HEALTHY, nodePoolFromRow(&row, nodePoolRuntime{nodes: 2, ready: 2}).GetStatus())
	row.Status = pgtype.Text{}
	assert.Equal(t, organizationv1.NodePoolStatus_NODE_POOL_STATUS_UNSPECIFIED, nodePoolFromRow(&row, nodePoolRuntime{}).GetStatus())
}

// While a new shoot has no Prometheus, a pool falls back to the node counts
// cluster-worker read from the shoot's API server, as long as they are fresh.
func TestRuntimeForPoolFallsBackToStoredCounts(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	stored := func(nodes, ready int32, age time.Duration) db.NodePoolListByClusterIDRow {
		return db.NodePoolListByClusterIDRow{
			Name:             "small",
			NodeCount:        pgtype.Int4{Int32: nodes, Valid: true},
			NodeCountReady:   pgtype.Int4{Int32: ready, Valid: true},
			NodeCountUpdated: pgtype.Timestamptz{Time: now.Add(-age), Valid: true},
		}
	}
	prometheus := map[string]nodePoolRuntime{
		kubename.GenerateWorkerName("small"): {nodes: 3, ready: 3, version: "v1.33.4"},
	}

	t.Run("Prometheus wins when it reports the pool", func(t *testing.T) {
		t.Parallel()
		row := stored(2, 2, time.Minute)
		assert.Equal(t, nodePoolRuntime{nodes: 3, ready: 3, version: "v1.33.4"}, runtimeForPool(prometheus, &row, now))
	})

	t.Run("fresh stored counts stand in, without a version", func(t *testing.T) {
		t.Parallel()
		row := stored(2, 1, storedNodeCountMaxAge-time.Second)
		runtime := runtimeForPool(nil, &row, now)
		assert.Equal(t, nodePoolRuntime{nodes: 2, ready: 1}, runtime)

		row.Status = pgtype.Text{String: "ready", Valid: true}
		pool := nodePoolFromRow(&row, runtime)
		assert.Equal(t, int32(2), pool.GetCurrentNodes())
		assert.Equal(t, organizationv1.NodePoolStatus_NODE_POOL_STATUS_DEGRADED, pool.GetStatus())
		assert.Empty(t, pool.GetVersion())
	})

	t.Run("an unreachable Prometheus on a settled cluster reads as unknown", func(t *testing.T) {
		t.Parallel()
		// cluster-worker stopped recording the counts once the pools
		// settled; a Prometheus outage after that must not show them.
		row := stored(2, 2, storedNodeCountMaxAge)
		row.Status = pgtype.Text{String: "ready", Valid: true}
		runtime := runtimeForPool(nil, &row, now)
		assert.Equal(t, nodePoolRuntime{}, runtime)

		pool := nodePoolFromRow(&row, runtime)
		assert.Zero(t, pool.GetCurrentNodes())
		assert.Equal(t, organizationv1.NodePoolStatus_NODE_POOL_STATUS_UNSPECIFIED, pool.GetStatus())
	})

	t.Run("no stored counts read as unknown", func(t *testing.T) {
		t.Parallel()
		row := db.NodePoolListByClusterIDRow{Name: "small"}
		assert.Equal(t, nodePoolRuntime{}, runtimeForPool(nil, &row, now))
	})
}
