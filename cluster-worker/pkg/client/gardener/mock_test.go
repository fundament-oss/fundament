package gardener

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The mock reports a cluster each time its time-based status changes, and
// once more when its shoot is gone, the way the real client's watch does.
func TestMockNotifyStatusChanges(t *testing.T) {
	t.Parallel()

	m := NewMock(slog.New(slog.NewTextHandler(io.Discard, nil)))
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	m.SetClock(func() time.Time { return now })
	var reported []uuid.UUID
	m.SetStatusChangeHandler(func(id uuid.UUID) { reported = append(reported, id) })

	cluster := testCluster()
	_, err := m.ApplyShoot(t.Context(), cluster)
	require.NoError(t, err)

	m.NotifyStatusChanges()
	assert.Equal(t, []uuid.UUID{cluster.ID}, reported, "a new shoot is reported")

	reported = nil
	m.NotifyStatusChanges()
	assert.Empty(t, reported, "nothing changed")

	now = now.Add(m.ProgressingDelay + m.ReadyDelay)
	m.NotifyStatusChanges()
	assert.Equal(t, []uuid.UUID{cluster.ID}, reported, "the change to ready is reported")

	reported = nil
	require.NoError(t, m.DeleteShootByClusterID(t.Context(), cluster.ID))
	now = now.Add(m.DeleteDelay)
	status, err := m.GetShootStatus(t.Context(), cluster)
	require.NoError(t, err)
	require.Equal(t, MsgShootNotFound, status.Message)
	m.NotifyStatusChanges()
	assert.Equal(t, []uuid.UUID{cluster.ID}, reported, "a shoot that is gone is reported")
}

// A mock create runs through the steps a real one reports, in order: the
// create tasks, ready within Gardener's grace period, ready and unhealthy
// until the conditions pass, then ready and healthy.
func TestMockCreateFollowsGardener(t *testing.T) {
	t.Parallel()

	m := NewMock(slog.New(slog.NewTextHandler(io.Discard, nil)))
	start := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	now := start
	m.SetClock(func() time.Time { return now })
	cluster := testCluster()
	_, err := m.ApplyShoot(t.Context(), cluster)
	require.NoError(t, err)

	at := func(d time.Duration) *ShootStatus {
		now = start.Add(d)
		status, err := m.GetShootStatus(t.Context(), cluster)
		require.NoError(t, err)
		return status
	}
	progressing := func(task string) *ShootStatus {
		return &ShootStatus{Status: StatusProgressing, Message: "Create: " + task, Operation: OperationCreate}
	}

	assert.Equal(t, progressing("Reconciliation of Shoot cluster initialized."), at(0))
	assert.Equal(t, progressing("Waiting until Kubernetes API server rolled out"), at(2*time.Second))
	assert.Equal(t, progressing("Waiting until gardener-resource-manager reports readiness"), at(4*time.Second))
	assert.Equal(t, progressing("Waiting until shoot worker nodes have been reconciled"), at(5500*time.Millisecond))
	assert.Equal(t, &ShootStatus{Status: StatusReady, Message: MsgShootUnhealthy, Operation: OperationCreate, HealthGrace: true}, at(6*time.Second))
	assert.Equal(t, &ShootStatus{Status: StatusReady, Message: MsgShootUnhealthy, Operation: OperationCreate}, at(7*time.Second))
	assert.Equal(t, &ShootStatus{Status: StatusReady, Message: MsgShootReady, Operation: OperationCreate, Healthy: true}, at(9*time.Second))
}
