package cluster

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/common/dbconst"
	"github.com/fundament-oss/fundament/common/kubename"
)

// Gardener names machine deployments after the generated worker name, so the
// fixtures must carry kubename.GenerateWorkerName(pool), never the raw pool
// name — production text never contains the raw name.
func enrMessage(pool string) string {
	return `machine deployment "shoot--acme--tf-a-` + kubename.GenerateWorkerName(pool) + `-z1" is waiting for machines to join`
}

func TestNextNodePoolStatuses(t *testing.T) {
	t.Parallel()

	enrNomach := enrMessage("nomach")
	stuck := &gardener.ShootStatus{
		Status: gardener.StatusProgressing, Operation: gardener.OperationReconcile,
		Message:               "Reconcile: Waiting until worker resource status is updated",
		EveryNodeReadyMessage: enrNomach,
	}
	quietReconcile := &gardener.ShootStatus{
		Status: gardener.StatusProgressing, Operation: gardener.OperationReconcile,
		Message: "Reconcile: 10% done",
	}
	readyHealthy := &gardener.ShootStatus{Status: gardener.StatusReady, Healthy: true}

	tests := []struct {
		name     string
		stored   []storedNodePool
		observed *gardener.ShootStatus
		pushed   bool
		want     []nodePoolUpdate
	}{
		{
			name:     "named pool waits; settled pool keeps its status; new pool progresses",
			stored:   []storedNodePool{{Name: "extra", Status: "ready"}, {Name: "fresh"}, {Name: "nomach", Status: "progressing"}},
			observed: stuck,
			pushed:   true,
			want: []nodePoolUpdate{
				{Name: "fresh", Status: dbconst.NodePoolStatus_Progressing},
				{Name: "nomach", Status: dbconst.NodePoolStatus_Waiting, Message: enrNomach,
					Events: []statusEvent{{Type: dbconst.ClusterEventEventType_NodepoolWaiting, Message: "Node pool nomach: " + enrNomach}}},
			},
		},
		{
			name:     "raw pool name in the text does not match",
			stored:   []storedNodePool{{Name: "nomach", Status: "ready"}},
			observed: &gardener.ShootStatus{Status: gardener.StatusProgressing, Operation: gardener.OperationReconcile, EveryNodeReadyMessage: `machine deployment "shoot--acme--tf-a-nomach-z1" is waiting`},
			want:     nil,
		},
		{
			name:     "routine reconcile leaves settled pools alone",
			stored:   []storedNodePool{{Name: "web", Status: "ready"}, {Name: "db", Status: "ready"}},
			observed: quietReconcile,
			want:     nil,
		},
		{
			name:     "a rollout meeting unclassified pools mid-reconcile stays silent",
			stored:   []storedNodePool{{Name: "legacy"}},
			observed: quietReconcile,
			want:     nil,
		},
		{
			name:     "a new pool is progressing while its change is still being applied",
			stored:   []storedNodePool{{Name: "fresh"}},
			observed: readyHealthy,
			pushed:   true,
			want:     []nodePoolUpdate{{Name: "fresh", Status: dbconst.NodePoolStatus_Progressing}},
		},
		{
			name:     "first classification of a settled pool is silent",
			stored:   []storedNodePool{{Name: "extra", Status: ""}},
			observed: readyHealthy,
			want:     []nodePoolUpdate{{Name: "extra", Status: dbconst.NodePoolStatus_Ready}},
		},
		{
			name:   "a cleared complaint returns a stuck pool to progressing",
			stored: []storedNodePool{{Name: "fixed", Status: "waiting", Message: enrMessage("fixed")}, {Name: "nomach", Status: "waiting", Message: enrNomach}},
			observed: &gardener.ShootStatus{
				Status: gardener.StatusProgressing, Operation: gardener.OperationReconcile,
				EveryNodeReadyMessage: enrNomach, // names nomach only; fixed recovered
			},
			want: []nodePoolUpdate{
				{Name: "fixed", Status: dbconst.NodePoolStatus_Progressing},
			},
		},
		{
			name:     "a stuck pool keeps its reason while the report carries no detail",
			stored:   []storedNodePool{{Name: "nomach", Status: "waiting", Message: enrNomach}},
			observed: quietReconcile,
			want:     nil,
		},
		{
			name:     "a changed reason refreshes the row without a second event",
			stored:   []storedNodePool{{Name: "nomach", Status: "waiting", Message: "old reason naming " + kubename.GenerateWorkerName("nomach")}},
			observed: stuck,
			want: []nodePoolUpdate{
				{Name: "nomach", Status: dbconst.NodePoolStatus_Waiting, Message: enrNomach},
			},
		},
		{
			name:     "recovery records nodepool_ready",
			stored:   []storedNodePool{{Name: "nomach", Status: "waiting", Message: enrNomach}},
			observed: readyHealthy,
			want: []nodePoolUpdate{
				{Name: "nomach", Status: dbconst.NodePoolStatus_Ready,
					Events: []statusEvent{{Type: dbconst.ClusterEventEventType_NodepoolReady, Message: "Node pool nomach: machines are ready"}}},
			},
		},
		{
			name:   "a pool never matches another pool's worker name",
			stored: []storedNodePool{{Name: "web", Status: "ready"}, {Name: "my-web", Status: "ready"}},
			observed: &gardener.ShootStatus{
				Status: gardener.StatusProgressing, Operation: gardener.OperationReconcile,
				EveryNodeReadyMessage: enrMessage("my-web"),
			},
			want: []nodePoolUpdate{
				{Name: "my-web", Status: dbconst.NodePoolStatus_Waiting,
					Message: enrMessage("my-web"),
					Events: []statusEvent{{Type: dbconst.ClusterEventEventType_NodepoolWaiting,
						Message: "Node pool my-web: " + enrMessage("my-web")}}},
			},
		},
		{
			name:     "non-pool unhealthiness leaves pools alone",
			stored:   []storedNodePool{{Name: "extra", Status: "ready"}},
			observed: &gardener.ShootStatus{Status: gardener.StatusReady, Healthy: false, Message: gardener.MsgShootUnhealthy},
			want:     nil,
		},
		{
			name:   "failed operation naming the pool records nodepool_error",
			stored: []storedNodePool{{Name: "nomach", Status: "waiting", Message: "old"}},
			observed: &gardener.ShootStatus{Status: gardener.StatusError,
				Message:    "Failed to reconcile",
				LastErrors: []string{enrMessage("nomach")}},
			want: []nodePoolUpdate{
				{Name: "nomach", Status: dbconst.NodePoolStatus_Error,
					Message: enrMessage("nomach"),
					Events: []statusEvent{{Type: dbconst.ClusterEventEventType_NodepoolError,
						Message: "Node pool nomach: " + enrMessage("nomach")}}},
			},
		},
		{
			name:     "status flips waiting to error with identical message still records nodepool_error",
			stored:   []storedNodePool{{Name: "nomach", Status: "waiting", Message: enrNomach}},
			observed: &gardener.ShootStatus{Status: gardener.StatusError, Message: "Failed to reconcile", LastErrors: []string{enrNomach}},
			want: []nodePoolUpdate{
				{Name: "nomach", Status: dbconst.NodePoolStatus_Error, Message: enrNomach,
					Events: []statusEvent{{Type: dbconst.ClusterEventEventType_NodepoolError, Message: "Node pool nomach: " + enrNomach}}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, nextNodePoolStatuses(tt.stored, tt.observed, tt.pushed))
		})
	}
}
