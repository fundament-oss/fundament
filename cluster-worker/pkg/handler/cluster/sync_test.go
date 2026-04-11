package cluster

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
	"github.com/fundament-oss/fundament/common/dbconst"
)

var (
	_ handler.SyncHandler      = (*Handler)(nil)
	_ handler.StatusHandler    = (*Handler)(nil)
	_ handler.ReconcileHandler = (*Handler)(nil)
)

func TestSyncMessage(t *testing.T) {
	tests := []struct {
		event      dbconst.ClusterOutboxEvent
		entityType handler.EntityType
		want       string
	}{
		{event: dbconst.ClusterOutboxEvent_Created, entityType: handler.EntityCluster, want: "Cluster created"},
		{event: dbconst.ClusterOutboxEvent_Updated, entityType: handler.EntityCluster, want: "Cluster updated"},
		{event: dbconst.ClusterOutboxEvent_Deleted, entityType: handler.EntityCluster, want: "Cluster deleted"},
		{event: dbconst.ClusterOutboxEvent_Reconcile, entityType: handler.EntityCluster, want: "Cluster reconciled"},
		{event: dbconst.ClusterOutboxEvent_Created, entityType: handler.EntityNodePool, want: "Node pool created"},
		{event: dbconst.ClusterOutboxEvent_Updated, entityType: handler.EntityNodePool, want: "Node pool updated"},
		{event: dbconst.ClusterOutboxEvent_Deleted, entityType: handler.EntityNodePool, want: "Node pool deleted"},
		{event: dbconst.ClusterOutboxEvent_Reconcile, entityType: handler.EntityNodePool, want: "Node pool reconciled"},
	}
	for _, tt := range tests {
		t.Run(string(tt.event)+"_"+string(tt.entityType), func(t *testing.T) {
			got := syncMessage(tt.event, tt.entityType)
			if got != tt.want {
				t.Errorf("syncMessage(%q, %q) = %q, want %q", tt.event, tt.entityType, got, tt.want)
			}
		})
	}
}

func TestSyncMessage_PanicsOnUnknownEvent(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for unknown event, got none")
		}
	}()
	syncMessage("unknown_event", handler.EntityCluster)
}

func TestToGardenerNodePools(t *testing.T) {
	rows := []db.NodePoolListByClusterIDRow{
		{
			ID:           uuid.New(),
			Name:         "worker-1",
			MachineType:  "n1-standard-4",
			AutoscaleMin: 1,
			AutoscaleMax: 5,
			Created:      pgtype.Timestamptz{Valid: true},
		},
		{
			ID:           uuid.New(),
			Name:         "worker-2",
			MachineType:  "n1-standard-8",
			AutoscaleMin: 2,
			AutoscaleMax: 10,
			Created:      pgtype.Timestamptz{Valid: true},
		},
	}

	pools := toGardenerNodePools(rows)

	if len(pools) != 2 {
		t.Fatalf("expected 2 pools, got %d", len(pools))
	}

	for i, want := range []gardener.NodePool{
		{Name: "worker-1", MachineType: "n1-standard-4", AutoscaleMin: 1, AutoscaleMax: 5},
		{Name: "worker-2", MachineType: "n1-standard-8", AutoscaleMin: 2, AutoscaleMax: 10},
	} {
		got := pools[i]
		if got != want {
			t.Errorf("pool[%d] = %+v, want %+v", i, got, want)
		}
	}
}

func TestToGardenerNodePools_Empty(t *testing.T) {
	pools := toGardenerNodePools(nil)
	if len(pools) != 0 {
		t.Errorf("expected 0 pools, got %d", len(pools))
	}
}
