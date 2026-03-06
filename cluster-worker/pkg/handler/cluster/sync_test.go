package cluster

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/google/uuid"
)

var (
	_ handler.SyncHandler      = (*Handler)(nil)
	_ handler.StatusHandler    = (*Handler)(nil)
	_ handler.ReconcileHandler = (*Handler)(nil)
)

func TestSyncMessage(t *testing.T) {
	tests := []struct {
		event  string
		source string
		want   string
	}{
		{event: "created", source: "cluster", want: "Cluster created"},
		{event: "updated", source: "cluster", want: "Cluster updated"},
		{event: "deleted", source: "cluster", want: "Cluster deleted"},
		{event: "reconcile", source: "cluster", want: "Cluster reconciled"},
		{event: "created", source: "node_pool", want: "Node pool created"},
		{event: "updated", source: "node_pool", want: "Node pool updated"},
		{event: "deleted", source: "node_pool", want: "Node pool deleted"},
		{event: "reconcile", source: "node_pool", want: "Node pool reconciled"},
	}
	for _, tt := range tests {
		t.Run(tt.event+"_"+tt.source, func(t *testing.T) {
			got := syncMessage(tt.event, tt.source)
			if got != tt.want {
				t.Errorf("syncMessage(%q, %q) = %q, want %q", tt.event, tt.source, got, tt.want)
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
	syncMessage("unknown_event", "cluster")
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
