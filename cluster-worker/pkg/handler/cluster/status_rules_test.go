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
			observed: &gardener.ShootStatus{Status: gardener.StatusProgressing, Message: "Reconcile: Syncing", Operation: gardener.OperationReconcile},
			want:     shootStatusUpdate{Status: gardener.StatusReady, Message: gardener.MsgShootReady, Health: healthy},
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
