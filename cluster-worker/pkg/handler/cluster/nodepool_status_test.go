package cluster

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/common/dbconst"
)

func TestNextNodePoolStatuses(t *testing.T) {
	t.Parallel()

	enrNomach := `machine deployment "shoot--acme--tf-a-nomach-z1" is waiting for machines to join`
	stuck := &gardener.ShootStatus{
		Status: gardener.StatusProgressing, Operation: gardener.OperationReconcile,
		Message:               "Reconcile: Waiting until worker resource status is updated",
		EveryNodeReadyMessage: enrNomach,
	}
	readyHealthy := &gardener.ShootStatus{Status: gardener.StatusReady, Healthy: true}

	tests := []struct {
		name     string
		stored   []storedNodePool
		observed *gardener.ShootStatus
		want     []nodePoolUpdate
	}{
		{
			name:     "named pool waits, unnamed pool progresses",
			stored:   []storedNodePool{{Name: "extra", Status: "ready"}, {Name: "nomach", Status: "progressing"}},
			observed: stuck,
			want: []nodePoolUpdate{
				{Name: "extra", Status: dbconst.NodePoolStatus_Progressing},
				{Name: "nomach", Status: dbconst.NodePoolStatus_Waiting, Message: enrNomach,
					Events: []statusEvent{{Type: dbconst.ClusterEventEventType_NodepoolWaiting, Message: "Node pool nomach: " + enrNomach}}},
			},
		},
		{
			name:     "same waiting message writes no second event",
			stored:   []storedNodePool{{Name: "nomach", Status: "waiting", Message: enrNomach}},
			observed: stuck,
			want:     nil,
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
			name:     "first classification of a settled pool is silent",
			stored:   []storedNodePool{{Name: "extra", Status: ""}},
			observed: readyHealthy,
			want:     []nodePoolUpdate{{Name: "extra", Status: dbconst.NodePoolStatus_Ready}},
		},
		{
			name:   "pool name prefixing another is not matched",
			stored: []storedNodePool{{Name: "web", Status: "ready"}, {Name: "web2", Status: "ready"}},
			observed: &gardener.ShootStatus{
				Status: gardener.StatusProgressing, Operation: gardener.OperationReconcile,
				EveryNodeReadyMessage: `machine deployment "shoot--acme--tf-a-web2-z1" is waiting`,
			},
			want: []nodePoolUpdate{
				{Name: "web", Status: dbconst.NodePoolStatus_Progressing},
				{Name: "web2", Status: dbconst.NodePoolStatus_Waiting,
					Message: `machine deployment "shoot--acme--tf-a-web2-z1" is waiting`,
					Events: []statusEvent{{Type: dbconst.ClusterEventEventType_NodepoolWaiting,
						Message: `Node pool web2: machine deployment "shoot--acme--tf-a-web2-z1" is waiting`}}},
			},
		},
		{
			name:   "non-pool unhealthiness leaves pools alone",
			stored: []storedNodePool{{Name: "extra", Status: "ready"}},
			observed: &gardener.ShootStatus{Status: gardener.StatusReady, Healthy: false,
				Message: gardener.MsgShootUnhealthy},
			want: nil,
		},
		{
			name:   "failed operation naming the pool records nodepool_error",
			stored: []storedNodePool{{Name: "nomach", Status: "waiting", Message: "old"}},
			observed: &gardener.ShootStatus{Status: gardener.StatusError,
				Message:    "Failed to reconcile",
				LastErrors: []string{`machine deployment "shoot--acme--tf-a-nomach-z1" failed to create machines`}},
			want: []nodePoolUpdate{
				{Name: "nomach", Status: dbconst.NodePoolStatus_Error,
					Message: `machine deployment "shoot--acme--tf-a-nomach-z1" failed to create machines`,
					Events: []statusEvent{{Type: dbconst.ClusterEventEventType_NodepoolError,
						Message: `Node pool nomach: machine deployment "shoot--acme--tf-a-nomach-z1" failed to create machines`}}},
			},
		},
		{
			name:   "status flips waiting to error with identical message still records nodepool_error",
			stored: []storedNodePool{{Name: "nomach", Status: "waiting", Message: enrNomach}},
			observed: &gardener.ShootStatus{Status: gardener.StatusError,
				Message:    "Failed to reconcile",
				LastErrors: []string{enrNomach}},
			want: []nodePoolUpdate{
				{Name: "nomach", Status: dbconst.NodePoolStatus_Error,
					Message: enrNomach,
					Events: []statusEvent{{Type: dbconst.ClusterEventEventType_NodepoolError,
						Message: "Node pool nomach: " + enrNomach}}},
			},
		},
		{
			name:     "pending shoot leaves pools untouched",
			stored:   []storedNodePool{{Name: "extra", Status: ""}},
			observed: &gardener.ShootStatus{Status: gardener.StatusPending, Message: gardener.MsgShootNotFound},
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, nextNodePoolStatuses(tt.stored, tt.observed))
		})
	}
}
