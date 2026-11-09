package provider

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func clusterDetails(status organizationv1.ClusterStatus, outboxStatus string, updating bool) *organizationv1.ClusterDetails {
	sync := organizationv1.SyncState_builder{OutboxError: new("shoot rejected"), ShootUpdating: updating}
	if outboxStatus != "" {
		sync.OutboxStatus = new(outboxStatus)
	}
	return organizationv1.ClusterDetails_builder{
		Status:    status,
		SyncState: sync.Build(),
	}.Build()
}

func TestClusterRunning(t *testing.T) {
	tests := []struct {
		name         string
		status       organizationv1.ClusterStatus
		outboxStatus string
		updating     bool
		changed      bool
		done         bool
		errContains  string
	}{
		{name: "running and synced", status: organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING, outboxStatus: "completed", done: true},
		{name: "running, never synced through the outbox", status: organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING, done: true},
		{name: "running, change not yet applied", status: organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING, outboxStatus: "pending"},
		{name: "running, change being retried", status: organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING, outboxStatus: "retrying"},
		{name: "change applied, rolling out", status: organizationv1.ClusterStatus_CLUSTER_STATUS_UPGRADING, outboxStatus: "completed"},
		{name: "starting and synced", status: organizationv1.ClusterStatus_CLUSTER_STATUS_STARTING, outboxStatus: "completed", done: true},
		{name: "starting, change not yet applied", status: organizationv1.ClusterStatus_CLUSTER_STATUS_STARTING, outboxStatus: "pending"},
		{name: "unhealthy and synced", status: organizationv1.ClusterStatus_CLUSTER_STATUS_UNHEALTHY, outboxStatus: "completed", done: true},
		{name: "unhealthy, change not yet applied", status: organizationv1.ClusterStatus_CLUSTER_STATUS_UNHEALTHY, outboxStatus: "pending"},
		{name: "unhealthy while a rollout still runs", status: organizationv1.ClusterStatus_CLUSTER_STATUS_UNHEALTHY, outboxStatus: "completed", updating: true},
		{name: "starting while a rollout still runs", status: organizationv1.ClusterStatus_CLUSTER_STATUS_STARTING, outboxStatus: "completed", updating: true},
		{name: "provisioning", status: organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING, outboxStatus: "completed"},
		{name: "error that Gardener may retry", status: organizationv1.ClusterStatus_CLUSTER_STATUS_ERROR, outboxStatus: "completed"},
		{name: "sync given up", status: organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING, outboxStatus: "failed", errContains: "shoot rejected"},
		{name: "being deleted", status: organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING, outboxStatus: "completed", errContains: "deleting"},
		{name: "lost, change not yet applied", status: organizationv1.ClusterStatus_CLUSTER_STATUS_LOST, outboxStatus: "pending", changed: true},
		{name: "lost, node pool change not yet applied", status: organizationv1.ClusterStatus_CLUSTER_STATUS_LOST, outboxStatus: "completed", changed: true},
		{name: "lost with no change", status: organizationv1.ClusterStatus_CLUSTER_STATUS_LOST, outboxStatus: "completed", errContains: "cluster is lost"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			done, err := clusterRunning(clusterDetails(tt.status, tt.outboxStatus, tt.updating), tt.changed)
			if tt.errContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.done, done)
		})
	}
}

func TestSyncGivenUp(t *testing.T) {
	sync := clusterDetails(organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING, "retrying", false).GetSyncState()
	require.NoError(t, syncGivenUp(sync, "deleting the cluster"))

	sync = clusterDetails(organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING, "failed", false).GetSyncState()
	require.ErrorContains(t, syncGivenUp(sync, "deleting the cluster"), "gave up deleting the cluster in Gardener: shoot rejected")
}

func TestDescribeClusterState(t *testing.T) {
	cluster := clusterDetails(organizationv1.ClusterStatus_CLUSTER_STATUS_ERROR, "retrying", false)
	cluster.GetSyncState().SetShootMessage("no free machines")
	assert.Equal(t, "last status: error, no free machines; sync retrying: shoot rejected",
		describeClusterState(cluster.GetStatus(), cluster.GetSyncState()))
}

func TestWaitError(t *testing.T) {
	expired, cancelExpired := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelExpired()
	timedOut := waitError(expired, fmt.Errorf("poll: %w", context.DeadlineExceeded), "to run", "last status: error, no free machines")
	assert.Equal(t, "timed out waiting for the cluster to run (last status: error, no free machines)", timedOut.Error())
	assert.True(t, waitInconclusive(timedOut))

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	interrupted := waitError(canceled, context.Canceled, "to be deleted", "last status: deleting")
	assert.Equal(t, "interrupted while waiting for the cluster to be deleted (last status: deleting)", interrupted.Error())
	assert.True(t, waitInconclusive(interrupted))

	// An HTTP client timeout wraps context.DeadlineExceeded too, but the wait's own deadline has not passed.
	lostAPI := waitError(context.Background(), fmt.Errorf("polling cluster status: %w", context.DeadlineExceeded), "to run", "last status: provisioning")
	assert.Equal(t, "lost contact with Fundament while waiting for the cluster to run (last status: provisioning): polling cluster status: context deadline exceeded", lostAPI.Error())
	assert.True(t, waitInconclusive(lostAPI), "a lost API says nothing about the cluster")

	gaveUp := syncGivenUp(clusterDetails(organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING, "failed", false).GetSyncState(), "applying the cluster")
	assert.Equal(t, gaveUp, waitError(context.Background(), gaveUp, "to run", "last status: provisioning"))
	assert.False(t, waitInconclusive(gaveUp))

	_, deleting := clusterRunning(clusterDetails(organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING, "completed", false), true)
	assert.False(t, waitInconclusive(deleting))
}
