package cluster

import (
	"testing"
	"time"

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
				Status: gardener.StatusReady, Message: gardener.MsgShootAwaitingHealth, Health: unhealthy,
				Events:      []statusEvent{{Type: dbconst.ClusterEventEventType_StatusReady, Message: gardener.MsgShootAwaitingHealth}},
				InsertReady: true,
			},
		},
		{
			name:     "new cluster not healthy yet stays settling",
			stored:   storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootAwaitingHealth, Health: unhealthy},
			observed: readyUnhealthy,
			want:     shootStatusUpdate{Status: gardener.StatusReady, Message: gardener.MsgShootAwaitingHealth, Health: unhealthy},
		},
		{
			name:     "new cluster turns healthy",
			stored:   storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootAwaitingHealth, Health: unhealthy},
			observed: &gardener.ShootStatus{Status: gardener.StatusReady, Message: gardener.MsgShootReady, Operation: gardener.OperationCreate, Healthy: true},
			want: shootStatusUpdate{
				Status: gardener.StatusReady, Message: gardener.MsgShootReady, Health: healthy,
				Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusHealthy, Message: gardener.MsgShootReady}},
			},
		},
		{
			name:     "new cluster still not healthy at its first reconcile is unhealthy",
			stored:   storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootAwaitingHealth, Health: unhealthy},
			observed: &gardener.ShootStatus{Status: gardener.StatusReady, Message: gardener.MsgShootUnhealthy, Operation: gardener.OperationReconcile},
			want:     shootStatusUpdate{Status: gardener.StatusReady, Message: gardener.MsgShootUnhealthy, Health: unhealthy},
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
			name:     "healthy ready cluster degrades, also before its first reconcile",
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
				Status: gardener.StatusReady, Message: "machine not ready. Operation will be retried.", Health: healthy, Updating: true, Warning: true,
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
			name:     "grace period keeps a healthy ready cluster healthy",
			stored:   storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootReady, Health: healthy},
			observed: &gardener.ShootStatus{Status: gardener.StatusReady, Message: gardener.MsgShootUnhealthy, Operation: gardener.OperationReconcile, HealthGrace: true},
			want:     shootStatusUpdate{Status: gardener.StatusReady, Message: gardener.MsgShootReady, Health: healthy},
		},
		{
			name:     "grace period on a cluster never healthy is not healthy",
			stored:   storedShootState{Status: gardener.StatusProgressing, Message: "Create: Waiting"},
			observed: &gardener.ShootStatus{Status: gardener.StatusReady, Message: gardener.MsgShootUnhealthy, Operation: gardener.OperationCreate, HealthGrace: true},
			want: shootStatusUpdate{
				Status: gardener.StatusReady, Message: gardener.MsgShootAwaitingHealth, Health: unhealthy,
				Events:      []statusEvent{{Type: dbconst.ClusterEventEventType_StatusReady, Message: gardener.MsgShootAwaitingHealth}},
				InsertReady: true,
			},
		},
		{
			name:     "grace period during a reconcile keeps health and message",
			stored:   storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootReady, Health: healthy},
			observed: &gardener.ShootStatus{Status: gardener.StatusProgressing, Message: "Reconcile: Waiting until worker nodes are ready", Operation: gardener.OperationReconcile, HealthGrace: true},
			want:     shootStatusUpdate{Status: gardener.StatusReady, Message: gardener.MsgShootReady, Health: healthy},
		},
		{
			name:     "grace period during an update keeps health",
			stored:   storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootUpdatePending, Health: healthy, Updating: true},
			observed: &gardener.ShootStatus{Status: gardener.StatusProgressing, Message: "Reconcile: Waiting until worker nodes are ready", Operation: gardener.OperationReconcile, HealthGrace: true},
			want:     shootStatusUpdate{Status: gardener.StatusReady, Message: "Reconcile: Waiting until worker nodes are ready", Health: healthy, Updating: true},
		},
		{
			name:     "update finishing within the grace period stays healthy",
			stored:   storedShootState{Status: gardener.StatusReady, Message: "Reconcile: Waiting until worker nodes are ready", Health: healthy, Updating: true},
			observed: &gardener.ShootStatus{Status: gardener.StatusReady, Message: gardener.MsgShootUnhealthy, Operation: gardener.OperationReconcile, HealthGrace: true},
			want: shootStatusUpdate{
				Status: gardener.StatusReady, Message: gardener.MsgShootReady, Health: healthy,
				Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusReady, Message: gardener.MsgShootReady}},
			},
		},
		{
			name:     "grace period ends unhealthy",
			stored:   storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootReady, Health: healthy},
			observed: &gardener.ShootStatus{Status: gardener.StatusReady, Message: gardener.MsgShootUnhealthy, Operation: gardener.OperationReconcile},
			want: shootStatusUpdate{
				Status: gardener.StatusReady, Message: gardener.MsgShootUnhealthy, Health: unhealthy,
				Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusUnhealthy, Message: gardener.MsgShootUnhealthy}},
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
				Status: gardener.StatusProgressing, Message: "etcd not ready. Operation will be retried.", Warning: true,
				Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusWarning, Message: "etcd not ready. Operation will be retried."}},
			},
		},
		{
			name:     "same retried error again writes no second warning",
			stored:   storedShootState{Status: gardener.StatusProgressing, Message: "etcd not ready. Operation will be retried.", LastWarning: "etcd not ready. Operation will be retried."},
			observed: &gardener.ShootStatus{Status: gardener.StatusError, Message: "etcd not ready. Operation will be retried.", Operation: gardener.OperationCreate, Retrying: true},
			want:     shootStatusUpdate{Status: gardener.StatusProgressing, Message: "etcd not ready. Operation will be retried.", Warning: true},
		},
		{
			name:     "same retried error after progress in between writes no second warning",
			stored:   storedShootState{Status: gardener.StatusProgressing, Message: "Create: Deploying gardener-resource-manager", LastWarning: "etcd not ready. Operation will be retried."},
			observed: &gardener.ShootStatus{Status: gardener.StatusError, Message: "etcd not ready. Operation will be retried.", Operation: gardener.OperationCreate, Retrying: true},
			want:     shootStatusUpdate{Status: gardener.StatusProgressing, Message: "etcd not ready. Operation will be retried.", Warning: true},
		},
		{
			name:     "a different retried error is a new warning",
			stored:   storedShootState{Status: gardener.StatusProgressing, Message: "Create: Waiting", LastWarning: "etcd not ready. Operation will be retried."},
			observed: &gardener.ShootStatus{Status: gardener.StatusError, Message: "token not yet generated. Operation will be retried.", Operation: gardener.OperationCreate, Retrying: true},
			want: shootStatusUpdate{
				Status: gardener.StatusProgressing, Message: "token not yet generated. Operation will be retried.", Warning: true,
				Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusWarning, Message: "token not yet generated. Operation will be retried."}},
			},
		},
		{
			name:     "retried reconcile error keeps a ready cluster ready",
			stored:   storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootReady, Health: healthy},
			observed: &gardener.ShootStatus{Status: gardener.StatusError, Message: "Operation was aborted: seed is not yet ready", Operation: gardener.OperationReconcile, Retrying: true},
			want: shootStatusUpdate{
				Status: gardener.StatusReady, Message: "Operation was aborted: seed is not yet ready", Health: healthy, Warning: true,
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

func TestAddProgressEvent(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	const interval = time.Minute
	progressing := func(message string, events ...statusEvent) shootStatusUpdate {
		return shootStatusUpdate{Status: gardener.StatusProgressing, Message: message, Events: events}
	}
	progressEvent := func(message string) statusEvent {
		return statusEvent{Type: dbconst.ClusterEventEventType_StatusProgressing, Message: message}
	}

	tests := []struct {
		name       string
		update     shootStatusUpdate
		last       progressRecord
		wantEvents []statusEvent
		wantRetry  time.Duration
	}{
		{
			name:       "new message after the window records it",
			update:     progressing("Reconcile: Deploying kube-apiserver"),
			last:       progressRecord{At: now.Add(-2 * time.Minute), Message: "Reconcile: Waiting for etcd"},
			wantEvents: []statusEvent{progressEvent("Reconcile: Deploying kube-apiserver")},
		},
		{
			name:      "new message inside the window waits for the rest of it",
			update:    progressing("Reconcile: Deploying kube-apiserver"),
			last:      progressRecord{At: now.Add(-10 * time.Second), Message: "Reconcile: Waiting for etcd"},
			wantRetry: 50 * time.Second,
		},
		{
			name:   "message already recorded records nothing",
			update: progressing("Reconcile: Waiting until worker nodes are ready"),
			last:   progressRecord{At: now.Add(-25 * time.Minute), Message: "Reconcile: Waiting until worker nodes are ready"},
		},
		{
			name:       "no earlier progress event records it",
			update:     progressing("Create: Waiting for etcd"),
			wantEvents: []statusEvent{progressEvent("Create: Waiting for etcd")},
		},
		{
			name:       "a transition into progressing already records its message",
			update:     progressing("Create: Waiting for etcd", progressEvent("Create: Waiting for etcd")),
			wantEvents: []statusEvent{progressEvent("Create: Waiting for etcd")},
		},
		{
			name:       "a ready cluster being updated records progress",
			update:     shootStatusUpdate{Status: gardener.StatusReady, Updating: true, Message: "Reconcile: Waiting until worker nodes are ready"},
			last:       progressRecord{At: now.Add(-2 * time.Minute), Message: gardener.MsgShootUpdatePending},
			wantEvents: []statusEvent{progressEvent("Reconcile: Waiting until worker nodes are ready")},
		},
		{
			name:       "a retried error is recorded as the warning only",
			update:     shootStatusUpdate{Status: gardener.StatusReady, Updating: true, Warning: true, Message: "task errors: worker nodes not ready", Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusWarning, Message: "task errors: worker nodes not ready"}}},
			last:       progressRecord{At: now.Add(-5 * time.Minute), Message: "Reconcile: Waiting until shoot worker nodes have been reconciled"},
			wantEvents: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusWarning, Message: "task errors: worker nodes not ready"}},
		},
		{
			name:   "a repeated retried error records no progress either",
			update: shootStatusUpdate{Status: gardener.StatusProgressing, Warning: true, Message: "task errors: worker nodes not ready"},
			last:   progressRecord{At: now.Add(-5 * time.Minute), Message: "Create: Waiting until worker resource status is updated with latest machine deployments"},
		},
		{
			name:   "a ready cluster records no progress",
			update: shootStatusUpdate{Status: gardener.StatusReady, Message: "Reconcile: Waiting until system components are healthy"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			update := tt.update
			assert.Equal(t, tt.wantRetry, addProgressEvent(&update, tt.last, now, interval))
			assert.Equal(t, tt.wantEvents, update.Events)
		})
	}
}
