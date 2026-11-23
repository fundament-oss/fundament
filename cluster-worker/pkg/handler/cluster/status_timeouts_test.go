package cluster

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/common/dbconst"
)

func TestTimedOutPending(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	const limit = 20 * time.Minute
	notFound := &gardener.ShootStatus{Status: gardener.StatusPending, Message: gardener.MsgShootNotFound}

	tests := []struct {
		name     string
		stored   storedShootState
		observed *gardener.ShootStatus
		want     shootStatusUpdate
		wantOK   bool
	}{
		{
			name:     "a pending Shoot past the limit becomes error",
			stored:   storedShootState{Status: gardener.StatusPending, Message: gardener.MsgShootNotFound, Since: now.Add(-21 * time.Minute)},
			observed: notFound,
			want: shootStatusUpdate{
				Status: gardener.StatusError, Message: msgShootNeverAppeared,
				Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusError, Message: msgShootNeverAppeared}},
			},
			wantOK: true,
		},
		{
			name:     "at the limit it is still waiting",
			stored:   storedShootState{Status: gardener.StatusPending, Message: gardener.MsgShootNotFound, Since: now.Add(-limit)},
			observed: notFound,
		},
		{
			name:     "without a start time it is still waiting",
			stored:   storedShootState{Status: gardener.StatusPending, Message: gardener.MsgShootNotFound},
			observed: notFound,
		},
		{
			name:     "a cluster never checked does not time out",
			stored:   storedShootState{},
			observed: notFound,
		},
		{
			name:   "a Shoot Gardener is creating is progress, not absence",
			stored: storedShootState{Status: gardener.StatusPending, Message: "Component is not yet created", Since: now.Add(-time.Hour)},
			observed: &gardener.ShootStatus{
				Status: gardener.StatusPending, Message: "Create: Waiting until shoot is created",
			},
		},
		{
			name:     "a lost cluster is not this case",
			stored:   storedShootState{Status: gardener.StatusLost, Message: gardener.MsgShootLost, Since: now.Add(-time.Hour)},
			observed: notFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := timedOutPending(&tt.stored, tt.observed, now, limit)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

// A cluster timed out into error never had a Shoot, so a later check that
// still does not find one keeps the error: nothing disappeared.
func TestTimedOutPendingIsNotLost(t *testing.T) {
	t.Parallel()

	stored := storedShootState{Status: gardener.StatusError, Message: msgShootNeverAppeared}
	observed := &gardener.ShootStatus{Status: gardener.StatusPending, Message: gardener.MsgShootNotFound}

	assert.Equal(t, shootStatusUpdate{Status: gardener.StatusError, Message: msgShootNeverAppeared},
		nextShootStatus(&stored, observed))
}

func TestAddUpdateWarning(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	const limit = 30 * time.Minute

	tests := []struct {
		name    string
		stored  storedShootState
		update  shootStatusUpdate
		want    shootStatusUpdate
		wantNew bool
	}{
		{
			name:    "an update still rolling out after its sync settled warns",
			stored:  storedShootState{Status: gardener.StatusReady, UpdateSettled: now.Add(-31 * time.Minute)},
			update:  shootStatusUpdate{Status: gardener.StatusReady, Updating: true},
			want:    shootStatusUpdate{Status: gardener.StatusReady, Updating: true, Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusWarning, Message: msgUpdateStalled}}},
			wantNew: true,
		},
		{
			name:   "inside the limit it stays quiet",
			stored: storedShootState{Status: gardener.StatusReady, UpdateSettled: now.Add(-29 * time.Minute)},
			update: shootStatusUpdate{Status: gardener.StatusReady, Updating: true},
			want:   shootStatusUpdate{Status: gardener.StatusReady, Updating: true},
		},
		{
			name:   "the same warning is recorded once",
			stored: storedShootState{Status: gardener.StatusReady, UpdateSettled: now.Add(-time.Hour), LastWarning: msgUpdateStalled},
			update: shootStatusUpdate{Status: gardener.StatusReady, Updating: true},
			want:   shootStatusUpdate{Status: gardener.StatusReady, Updating: true},
		},
		{
			name:   "a finished update is not stalled",
			stored: storedShootState{Status: gardener.StatusReady, Message: gardener.MsgShootReady, UpdateSettled: now.Add(-time.Hour)},
			update: shootStatusUpdate{Status: gardener.StatusReady, Message: gardener.MsgShootReady},
			want:   shootStatusUpdate{Status: gardener.StatusReady, Message: gardener.MsgShootReady},
		},
		{
			name:   "no settled sync means nothing to measure",
			stored: storedShootState{Status: gardener.StatusReady},
			update: shootStatusUpdate{Status: gardener.StatusReady, Updating: true},
			want:   shootStatusUpdate{Status: gardener.StatusReady, Updating: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			update := tt.update
			added := addUpdateWarning(&update, &tt.stored, now, limit)
			assert.Equal(t, tt.wantNew, added)
			assert.Equal(t, tt.want, update)
		})
	}
}

func TestAddDeletingWarning(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	const limit = 2 * time.Hour

	tests := []struct {
		name    string
		stored  storedShootState
		update  shootStatusUpdate
		want    shootStatusUpdate
		wantNew bool
	}{
		{
			name:    "a Shoot deleting past the limit warns",
			stored:  storedShootState{Status: gardener.StatusDeleting, Since: now.Add(-3 * time.Hour)},
			update:  shootStatusUpdate{Status: gardener.StatusDeleting, Message: "Delete: in progress"},
			want:    shootStatusUpdate{Status: gardener.StatusDeleting, Message: "Delete: in progress", Events: []statusEvent{{Type: dbconst.ClusterEventEventType_StatusWarning, Message: msgDeleteStuck}}},
			wantNew: true,
		},
		{
			name:   "inside the limit it stays quiet",
			stored: storedShootState{Status: gardener.StatusDeleting, Since: now.Add(-time.Hour)},
			update: shootStatusUpdate{Status: gardener.StatusDeleting, Message: "Delete: in progress"},
			want:   shootStatusUpdate{Status: gardener.StatusDeleting, Message: "Delete: in progress"},
		},
		{
			name:   "the same warning is recorded once",
			stored: storedShootState{Status: gardener.StatusDeleting, Since: now.Add(-3 * time.Hour), LastWarning: msgDeleteStuck},
			update: shootStatusUpdate{Status: gardener.StatusDeleting, Message: "Delete: in progress"},
			want:   shootStatusUpdate{Status: gardener.StatusDeleting, Message: "Delete: in progress"},
		},
		{
			name:   "a cluster that is not deleting is not this case",
			stored: storedShootState{Status: gardener.StatusReady, Since: now.Add(-3 * time.Hour)},
			update: shootStatusUpdate{Status: gardener.StatusReady, Message: gardener.MsgShootReady},
			want:   shootStatusUpdate{Status: gardener.StatusReady, Message: gardener.MsgShootReady},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			update := tt.update
			added := addDeletingWarning(&update, &tt.stored, now, limit)
			assert.Equal(t, tt.wantNew, added)
			assert.Equal(t, tt.want, update)
		})
	}
}
