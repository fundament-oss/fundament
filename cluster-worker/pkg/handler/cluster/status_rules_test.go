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
				Events: []statusEvent{
					{Type: dbconst.ClusterEventEventType_StatusReady, Message: gardener.MsgShootAwaitingHealth},
					// Turning ready announces the API server if nothing did before.
					{Type: dbconst.ClusterEventEventType_StatusApiAvailable, Message: msgAPIServerAvailable},
				},
				InsertReady:  true,
				APIAvailable: true,

				InsertAPIAvailable: true,
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
				Events: []statusEvent{
					{Type: dbconst.ClusterEventEventType_StatusReady, Message: gardener.MsgShootReady},
					// Turning ready announces the API server if nothing did before.
					{Type: dbconst.ClusterEventEventType_StatusApiAvailable, Message: msgAPIServerAvailable},
				},
				InsertReady:  true,
				APIAvailable: true,

				InsertAPIAvailable: true,
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
			// organization-api keys STARTING on the awaiting-health message, so a
			// reconcile mid-settle must not swap it for Gardener's progress text.
			name:     "routine reconcile during settle keeps the awaiting-health message",
			stored:   storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootAwaitingHealth, Health: unhealthy},
			observed: &gardener.ShootStatus{Status: gardener.StatusProgressing, Message: "Reconcile: 10% done", Operation: gardener.OperationReconcile},
			want:     shootStatusUpdate{Status: gardener.StatusReady, Message: gardener.MsgShootAwaitingHealth, Health: unhealthy},
		},
		{
			name:     "update during settle keeps the awaiting-health message",
			stored:   storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootAwaitingHealth, Health: unhealthy, Updating: true},
			observed: &gardener.ShootStatus{Status: gardener.StatusProgressing, Message: "Reconcile: Waiting until worker nodes are ready", Operation: gardener.OperationReconcile},
			want:     shootStatusUpdate{Status: gardener.StatusReady, Message: gardener.MsgShootAwaitingHealth, Health: unhealthy, Updating: true},
		},
		{
			name:     "update finishing mid-settle stays settling",
			stored:   storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootAwaitingHealth, Health: unhealthy, Updating: true},
			observed: &gardener.ShootStatus{Status: gardener.StatusReady, Message: gardener.MsgShootUnhealthy, Operation: gardener.OperationReconcile},
			want: shootStatusUpdate{
				Status: gardener.StatusReady, Message: gardener.MsgShootAwaitingHealth, Health: unhealthy,
				Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusReady, Message: gardener.MsgShootAwaitingHealth}},
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
				Events: []statusEvent{
					{Type: dbconst.ClusterEventEventType_StatusReady, Message: gardener.MsgShootAwaitingHealth},
					// Turning ready announces the API server if nothing did before.
					{Type: dbconst.ClusterEventEventType_StatusApiAvailable, Message: msgAPIServerAvailable},
				},
				InsertReady:  true,
				APIAvailable: true,

				InsertAPIAvailable: true,
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
				Status: gardener.StatusLost, Message: gardener.MsgShootLost,
				Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusLost, Message: gardener.MsgShootLost}},
			},
		},
		{
			name:     "shoot lost while being created records the loss",
			stored:   storedShootState{Status: gardener.StatusProgressing, Message: "Create: Waiting for etcd"},
			observed: &gardener.ShootStatus{Status: gardener.StatusPending, Message: gardener.MsgShootNotFound},
			want: shootStatusUpdate{
				Status: gardener.StatusLost, Message: gardener.MsgShootLost,
				Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusLost, Message: gardener.MsgShootLost}},
			},
		},
		{
			name:     "failed cluster whose shoot disappears records the loss",
			stored:   storedShootState{Status: gardener.StatusError, Message: "quota exceeded"},
			observed: &gardener.ShootStatus{Status: gardener.StatusPending, Message: gardener.MsgShootNotFound},
			want: shootStatusUpdate{
				Status: gardener.StatusLost, Message: gardener.MsgShootLost,
				Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusLost, Message: gardener.MsgShootLost}},
			},
		},
		{
			name:     "lost shoot still missing stays lost without a second event",
			stored:   storedShootState{Status: gardener.StatusLost, Message: gardener.MsgShootLost},
			observed: &gardener.ShootStatus{Status: gardener.StatusPending, Message: gardener.MsgShootNotFound},
			want:     shootStatusUpdate{Status: gardener.StatusLost, Message: gardener.MsgShootLost},
		},
		{
			name:     "updating cluster whose shoot disappears records the loss",
			stored:   storedShootState{Status: gardener.StatusReady, Message: "Reconcile: Syncing", Health: healthy, Updating: true},
			observed: &gardener.ShootStatus{Status: gardener.StatusPending, Message: gardener.MsgShootNotFound},
			want: shootStatusUpdate{
				Status: gardener.StatusLost, Message: gardener.MsgShootLost,
				Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusLost, Message: gardener.MsgShootLost}},
			},
		},
		{
			name:     "lost cluster whose shoot a change created again is being created",
			stored:   storedShootState{Status: gardener.StatusLost, Message: gardener.MsgShootLost},
			observed: &gardener.ShootStatus{Status: gardener.StatusProgressing, Message: "Create: Waiting", Operation: gardener.OperationCreate},
			want: shootStatusUpdate{
				Status: gardener.StatusProgressing, Message: "Create: Waiting",
				Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusProgressing, Message: "Create: Waiting"}},
			},
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

func TestNextShootStatus_APIServerAvailable(t *testing.T) {
	t.Parallel()

	creating := storedShootState{Status: gardener.StatusProgressing, Message: "Create: Deploying Kubernetes API server"}
	apiUp := &gardener.ShootStatus{
		Status: gardener.StatusProgressing, Message: "Create: Waiting until shoot worker nodes have been reconciled",
		Operation: gardener.OperationCreate, APIServerAvailable: true,
	}
	apiEvent := statusEvent{Type: dbconst.ClusterEventEventType_StatusApiAvailable, Message: msgAPIServerAvailable}

	t.Run("announced once while the cluster is being created", func(t *testing.T) {
		t.Parallel()
		got := nextShootStatus(creating, apiUp)
		assert.True(t, got.APIAvailable)
		assert.True(t, got.InsertAPIAvailable)
		assert.Contains(t, got.Events, apiEvent)
		assert.False(t, got.InsertReady, "the cluster is not ready yet")

		seen := creating
		seen.Message = apiUp.Message
		seen.APIAvailable = true
		again := nextShootStatus(seen, apiUp)
		assert.False(t, again.InsertAPIAvailable)
		assert.NotContains(t, again.Events, apiEvent)
		assert.True(t, again.unchanged(seen), "a second observation writes nothing")
	})

	t.Run("a ready cluster only records the column", func(t *testing.T) {
		t.Parallel()
		stored := storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootReady, Health: dbconst.ClusterShootHealth_Healthy}
		observed := &gardener.ShootStatus{
			Status: gardener.StatusReady, Message: gardener.MsgShootReady, Operation: gardener.OperationReconcile,
			Healthy: true, APIServerAvailable: true,
		}
		got := nextShootStatus(stored, observed)
		assert.True(t, got.APIAvailable)
		assert.False(t, got.InsertAPIAvailable, "access came with the ready fan-out")
		assert.Empty(t, got.Events)
		assert.False(t, got.unchanged(stored), "the column still has to be written")
	})

	t.Run("turning ready announces it when the condition has not yet", func(t *testing.T) {
		t.Parallel()
		readyNoCondition := &gardener.ShootStatus{Status: gardener.StatusReady, Message: gardener.MsgShootUnhealthy, Operation: gardener.OperationCreate}
		got := nextShootStatus(creating, readyNoCondition)
		assert.True(t, got.InsertReady)
		assert.True(t, got.InsertAPIAvailable, "api_available is the only event that provisions access")
		assert.True(t, got.APIAvailable)
	})

	t.Run("recovering to ready after access was set up announces nothing new", func(t *testing.T) {
		t.Parallel()
		stored := storedShootState{Status: gardener.StatusError, Message: "quota exceeded", APIAvailable: true}
		got := nextShootStatus(stored, &gardener.ShootStatus{Status: gardener.StatusReady, Message: gardener.MsgShootReady, Healthy: true, APIServerAvailable: true})
		assert.True(t, got.InsertReady)
		assert.False(t, got.InsertAPIAvailable)
		assert.NotContains(t, got.Events, apiEvent)
	})

	t.Run("an API server that flaps keeps the first record", func(t *testing.T) {
		t.Parallel()
		seen := creating
		seen.APIAvailable = true
		flapped := *apiUp
		flapped.APIServerAvailable = false
		got := nextShootStatus(seen, &flapped)
		assert.False(t, got.APIAvailable)
		assert.False(t, got.APIGone)
	})

	t.Run("a lost or deleting Shoot forgets it", func(t *testing.T) {
		t.Parallel()
		seen := storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootReady, APIAvailable: true}
		for _, observed := range []*gardener.ShootStatus{
			{Status: gardener.StatusPending, Message: gardener.MsgShootNotFound},
			{Status: gardener.StatusDeleting, Message: "Shoot is being deleted"},
		} {
			got := nextShootStatus(seen, observed)
			assert.True(t, got.APIGone, observed.Status)
		}
	})

	t.Run("a lost cluster created again announces its API server and readiness again", func(t *testing.T) {
		t.Parallel()
		lost := storedShootState{Status: gardener.StatusLost, Message: gardener.MsgShootLost}
		got := nextShootStatus(lost, apiUp)
		assert.True(t, got.InsertAPIAvailable)
		assert.Contains(t, got.Events, apiEvent)

		ready := nextShootStatus(lost, &gardener.ShootStatus{Status: gardener.StatusReady, Message: gardener.MsgShootReady, Operation: gardener.OperationCreate, Healthy: true, APIServerAvailable: true})
		assert.True(t, ready.InsertReady)
		assert.True(t, ready.InsertAPIAvailable)
	})
}
