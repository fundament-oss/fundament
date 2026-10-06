package cluster

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/common/dbconst"
)

func TestNextShootStatus(t *testing.T) {
	t.Parallel()

	const (
		healthy   = dbconst.ClusterShootHealth_Healthy
		unhealthy = dbconst.ClusterShootHealth_Unhealthy
	)
	readyHealthy := &gardener.ShootStatus{Status: gardener.StatusReady, Message: gardener.MsgShootReady, Operation: gardener.OperationReconcile, Healthy: true}
	readyUnhealthy := &gardener.ShootStatus{Status: gardener.StatusReady, Message: gardener.MsgShootUnhealthy, Operation: gardener.OperationCreate}

	tests := []struct {
		name     string
		stored   storedShootState
		observed *gardener.ShootStatus
		want     shootStatusUpdate
	}{
		{
			name:     "create succeeds while conditions settle",
			stored:   storedShootState{Status: gardener.StatusProgressing, Message: "Create: Waiting"},
			observed: readyUnhealthy,
			want: shootStatusUpdate{
				Status: gardener.StatusReady, Message: gardener.MsgShootUnhealthy, Health: unhealthy,
				Events:      []statusEvent{{Type: dbconst.ClusterEventEventType_StatusReady, Message: gardener.MsgShootUnhealthy}},
				InsertReady: true,
			},
		},
		{
			name:     "conditions settle after ready",
			stored:   storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootUnhealthy, Health: unhealthy},
			observed: readyHealthy,
			want: shootStatusUpdate{
				Status: gardener.StatusReady, Message: gardener.MsgShootReady, Health: healthy,
				Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusHealthy, Message: gardener.MsgShootReady}},
			},
		},
		{
			name:     "healthy ready cluster degrades",
			stored:   storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootReady, Health: healthy},
			observed: readyUnhealthy,
			want: shootStatusUpdate{
				Status: gardener.StatusReady, Message: gardener.MsgShootUnhealthy, Health: unhealthy,
				Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusUnhealthy, Message: gardener.MsgShootUnhealthy}},
			},
		},
		{
			name:     "unchanged healthy ready cluster writes no event",
			stored:   storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootReady, Health: healthy},
			observed: readyHealthy,
			want:     shootStatusUpdate{Status: gardener.StatusReady, Message: gardener.MsgShootReady, Health: healthy},
		},
		{
			name:     "first recorded health on a ready row writes no event",
			stored:   storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootUnhealthy},
			observed: readyHealthy,
			want:     shootStatusUpdate{Status: gardener.StatusReady, Message: gardener.MsgShootReady, Health: healthy},
		},
		{
			name:     "routine reconcile keeps the row",
			stored:   storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootReady, Health: healthy},
			observed: &gardener.ShootStatus{Status: gardener.StatusProgressing, Message: "Reconcile: Syncing", Operation: gardener.OperationReconcile, Healthy: true},
			want:     shootStatusUpdate{Status: gardener.StatusReady, Message: gardener.MsgShootReady, Health: healthy},
		},
		{
			name:     "reconcile that breaks a condition records unhealthy",
			stored:   storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootReady, Health: healthy},
			observed: &gardener.ShootStatus{Status: gardener.StatusProgressing, Message: "Reconcile: Waiting until system components are healthy", Operation: gardener.OperationReconcile},
			want: shootStatusUpdate{
				Status: gardener.StatusReady, Message: "Reconcile: Waiting until system components are healthy", Health: unhealthy,
				Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusUnhealthy, Message: "Reconcile: Waiting until system components are healthy"}},
			},
		},
		{
			name:     "unhealthy reconcile follows Gardener's progress without a second event",
			stored:   storedShootState{Status: gardener.StatusReady, Message: "Reconcile: Waiting until system components are healthy", Health: unhealthy},
			observed: &gardener.ShootStatus{Status: gardener.StatusProgressing, Message: "Reconcile: Waiting until worker nodes are ready", Operation: gardener.OperationReconcile},
			want:     shootStatusUpdate{Status: gardener.StatusReady, Message: "Reconcile: Waiting until worker nodes are ready", Health: unhealthy},
		},
		{
			name:     "conditions recover during a reconcile",
			stored:   storedShootState{Status: gardener.StatusReady, Message: "Reconcile: Waiting until system components are healthy", Health: unhealthy},
			observed: &gardener.ShootStatus{Status: gardener.StatusProgressing, Message: "Reconcile: Syncing", Operation: gardener.OperationReconcile, Healthy: true},
			want: shootStatusUpdate{
				Status: gardener.StatusReady, Message: gardener.MsgShootReady, Health: healthy,
				Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusHealthy, Message: gardener.MsgShootReady}},
			},
		},
		{
			name:     "create in progress is not sticky",
			stored:   storedShootState{Status: gardener.StatusPending, Message: gardener.MsgShootNotFound},
			observed: &gardener.ShootStatus{Status: gardener.StatusProgressing, Message: "Create: Waiting", Operation: gardener.OperationCreate},
			want: shootStatusUpdate{
				Status: gardener.StatusProgressing, Message: "Create: Waiting",
				Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusProgressing, Message: "Create: Waiting"}},
			},
		},
		{
			name:     "migrate on a ready cluster is a lifecycle change",
			stored:   storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootReady, Health: healthy},
			observed: &gardener.ShootStatus{Status: gardener.StatusProgressing, Message: "Migrate: Moving", Operation: gardener.OperationMigrate},
			want: shootStatusUpdate{
				Status: gardener.StatusProgressing, Message: "Migrate: Moving",
				Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusProgressing, Message: "Migrate: Moving"}},
			},
		},
		{
			name:     "failed reconcile on a ready cluster is an error",
			stored:   storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootReady, Health: healthy},
			observed: &gardener.ShootStatus{Status: gardener.StatusError, Message: "quota exceeded", Operation: gardener.OperationReconcile},
			want: shootStatusUpdate{
				Status: gardener.StatusError, Message: "quota exceeded",
				Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusError, Message: "quota exceeded"}},
			},
		},
		{
			name:     "recovery from error fires the ready row again",
			stored:   storedShootState{Status: gardener.StatusError, Message: "quota exceeded"},
			observed: readyHealthy,
			want: shootStatusUpdate{
				Status: gardener.StatusReady, Message: gardener.MsgShootReady, Health: healthy,
				Events:      []statusEvent{{Type: dbconst.ClusterEventEventType_StatusReady, Message: gardener.MsgShootReady}},
				InsertReady: true,
			},
		},
		{
			name:     "update waiting for the gardenlet keeps the cluster ready and updating",
			stored:   storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootUpdatePending, Health: healthy, Updating: true},
			observed: &gardener.ShootStatus{Status: gardener.StatusProgressing, Message: gardener.MsgShootUpdatePending, Operation: gardener.OperationReconcile, Healthy: true},
			want:     shootStatusUpdate{Status: gardener.StatusReady, Message: gardener.MsgShootUpdatePending, Health: healthy, Updating: true},
		},
		{
			name:     "update in progress shows Gardener's progress without health events",
			stored:   storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootUpdatePending, Health: healthy, Updating: true},
			observed: &gardener.ShootStatus{Status: gardener.StatusProgressing, Message: "Reconcile: Waiting until worker nodes are ready", Operation: gardener.OperationReconcile},
			want:     shootStatusUpdate{Status: gardener.StatusReady, Message: "Reconcile: Waiting until worker nodes are ready", Health: unhealthy, Updating: true},
		},
		{
			name:     "finished update records ready without a second fan-out",
			stored:   storedShootState{Status: gardener.StatusReady, Message: "Reconcile: Waiting until worker nodes are ready", Health: unhealthy, Updating: true},
			observed: readyHealthy,
			want: shootStatusUpdate{
				Status: gardener.StatusReady, Message: gardener.MsgShootReady, Health: healthy,
				Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusReady, Message: gardener.MsgShootReady}},
			},
		},
		{
			name:     "retried error during an update keeps the cluster updating",
			stored:   storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootUpdatePending, Health: healthy, Updating: true},
			observed: &gardener.ShootStatus{Status: gardener.StatusError, Message: "machine not ready. Operation will be retried.", Operation: gardener.OperationReconcile, Retrying: true},
			want: shootStatusUpdate{
				Status: gardener.StatusReady, Message: "machine not ready. Operation will be retried.", Health: healthy, Updating: true,
				Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusWarning, Message: "machine not ready. Operation will be retried."}},
			},
		},
		{
			name:     "failed update ends the update",
			stored:   storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootUpdatePending, Health: healthy, Updating: true},
			observed: &gardener.ShootStatus{Status: gardener.StatusError, Message: "quota exceeded", Operation: gardener.OperationReconcile},
			want: shootStatusUpdate{
				Status: gardener.StatusError, Message: "quota exceeded",
				Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusError, Message: "quota exceeded"}},
			},
		},
		{
			name:     "ready cluster whose shoot disappears records the loss",
			stored:   storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootReady, Health: healthy},
			observed: &gardener.ShootStatus{Status: gardener.StatusPending, Message: gardener.MsgShootNotFound},
			want: shootStatusUpdate{
				Status: gardener.StatusPending, Message: gardener.MsgShootNotFound,
				Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusLost, Message: gardener.MsgShootNotFound}},
			},
		},
		{
			name:     "shoot lost while being created records the loss",
			stored:   storedShootState{Status: gardener.StatusProgressing, Message: "Create: Waiting for etcd"},
			observed: &gardener.ShootStatus{Status: gardener.StatusPending, Message: gardener.MsgShootNotFound},
			want: shootStatusUpdate{
				Status: gardener.StatusPending, Message: gardener.MsgShootNotFound,
				Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusLost, Message: gardener.MsgShootNotFound}},
			},
		},
		{
			name:     "failed cluster whose shoot disappears records the loss",
			stored:   storedShootState{Status: gardener.StatusError, Message: "quota exceeded"},
			observed: &gardener.ShootStatus{Status: gardener.StatusPending, Message: gardener.MsgShootNotFound},
			want: shootStatusUpdate{
				Status: gardener.StatusPending, Message: gardener.MsgShootNotFound,
				Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusLost, Message: gardener.MsgShootNotFound}},
			},
		},
		{
			name:     "lost shoot still missing records no second event",
			stored:   storedShootState{Status: gardener.StatusPending, Message: gardener.MsgShootNotFound},
			observed: &gardener.ShootStatus{Status: gardener.StatusPending, Message: gardener.MsgShootNotFound},
			want:     shootStatusUpdate{Status: gardener.StatusPending, Message: gardener.MsgShootNotFound},
		},
		{
			name:     "retried error during a create is a warning",
			stored:   storedShootState{Status: gardener.StatusProgressing, Message: "Create: Waiting"},
			observed: &gardener.ShootStatus{Status: gardener.StatusError, Message: "etcd not ready. Operation will be retried.", Operation: gardener.OperationCreate, Retrying: true},
			want: shootStatusUpdate{
				Status: gardener.StatusProgressing, Message: "etcd not ready. Operation will be retried.",
				Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusWarning, Message: "etcd not ready. Operation will be retried."}},
			},
		},
		{
			name:     "same retried error again writes no second warning",
			stored:   storedShootState{Status: gardener.StatusProgressing, Message: "etcd not ready. Operation will be retried."},
			observed: &gardener.ShootStatus{Status: gardener.StatusError, Message: "etcd not ready. Operation will be retried.", Operation: gardener.OperationCreate, Retrying: true},
			want:     shootStatusUpdate{Status: gardener.StatusProgressing, Message: "etcd not ready. Operation will be retried."},
		},
		{
			name:     "retried reconcile error keeps a ready cluster ready",
			stored:   storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootReady, Health: healthy},
			observed: &gardener.ShootStatus{Status: gardener.StatusError, Message: "Operation was aborted: seed is not yet ready", Operation: gardener.OperationReconcile, Retrying: true},
			want: shootStatusUpdate{
				Status: gardener.StatusReady, Message: "Operation was aborted: seed is not yet ready", Health: healthy,
				Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusWarning, Message: "Operation was aborted: seed is not yet ready"}},
			},
		},
		{
			name:     "never checked and shoot not visible yet",
			stored:   storedShootState{},
			observed: &gardener.ShootStatus{Status: gardener.StatusPending, Message: gardener.MsgShootNotFound},
			want:     shootStatusUpdate{Status: gardener.StatusPending, Message: gardener.MsgShootNotFound},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, nextShootStatus(tt.stored, tt.observed))
		})
	}
}
