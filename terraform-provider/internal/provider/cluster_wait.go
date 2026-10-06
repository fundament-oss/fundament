package provider

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// clusterPollInterval is a variable so tests can shorten it.
var clusterPollInterval = 10 * time.Second

const (
	clusterWaitMaxConsecutiveErrors = 5

	// The cluster outbox states, mirrored onto the cluster as sync_state.outbox_status.
	outboxCompleted = "completed"
	outboxFailed    = "failed"
)

// waitForClusterRunning polls GetCluster until the cluster runs and
// cluster-worker has applied every change to Gardener.
//
// A failed sync is fatal: cluster-worker has stopped retrying it. An error
// status is not: Gardener retries most errors, so it only ends the wait when
// the deadline passes, with Gardener's message in the error. It returns the
// cluster as last read, so callers need not read it again.
func waitForClusterRunning(ctx context.Context, client *FundamentClient, clusterID string) (*organizationv1.ClusterDetails, error) {
	var last *organizationv1.ClusterDetails

	err := pollWithBackoff(ctx, clusterPollInterval, clusterWaitMaxConsecutiveErrors, func(ctx context.Context) (bool, bool, error) {
		resp, err := client.ClusterService.GetCluster(ctx, organizationv1.GetClusterRequest_builder{
			ClusterId: clusterID,
		}.Build())
		if err != nil {
			return false, false, fmt.Errorf("polling cluster status: %w", err)
		}

		last = resp.GetCluster()
		done, err := clusterRunning(last)
		return done, err != nil, err
	})

	if err != nil {
		desc := "no status read"
		if last != nil {
			desc = describeClusterState(last.GetStatus(), last.GetSyncState())
		}
		return nil, waitError(ctx, err, "to run", desc)
	}
	return last, nil
}

// waitError says why a wait ended: its own deadline, an interruption, or an
// API that stopped answering, with the cluster's last status. It asks the
// wait's context, because an HTTP client timeout also reads as a deadline.
func waitError(ctx context.Context, err error, goal, desc string) error {
	var stuck *stuckError
	switch {
	case errors.As(err, &stuck):
		return err
	case errors.Is(ctx.Err(), context.Canceled):
		return &endedWaitError{msg: fmt.Sprintf("interrupted while waiting for the cluster %s (%s)", goal, desc), err: ctx.Err()}
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return &endedWaitError{msg: fmt.Sprintf("timed out waiting for the cluster %s (%s)", goal, desc), err: ctx.Err()}
	default:
		return fmt.Errorf("lost contact with Fundament while waiting for the cluster %s (%s): %w", goal, desc, err)
	}
}

// endedWaitError keeps the context error for errors.Is without printing
// "context deadline exceeded" to the user.
type endedWaitError struct {
	msg string
	err error
}

func (e *endedWaitError) Error() string { return e.msg }
func (e *endedWaitError) Unwrap() error { return e.err }

// stuckError is a wait that ended because the cluster cannot get where it
// should: Fundament gave up its sync, or the cluster is being deleted.
type stuckError struct{ msg string }

func (e *stuckError) Error() string { return e.msg }

// waitInconclusive reports whether a wait ended without learning that the
// cluster is stuck: it timed out, was interrupted, or lost the API. Fundament
// then carries on with the cluster.
func waitInconclusive(err error) bool {
	var stuck *stuckError
	return !errors.As(err, &stuck)
}

// clusterRunning reports whether the cluster runs with all changes applied, or
// an error once it cannot get there.
func clusterRunning(cluster *organizationv1.ClusterDetails) (bool, error) {
	sync := cluster.GetSyncState()
	if err := syncGivenUp(sync, "applying the cluster"); err != nil {
		return false, err
	}

	switch cluster.GetStatus() {
	case organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING:
		// The status is Gardener's last reading; a change still in the outbox has not reached it.
		return !sync.HasOutboxStatus() || sync.GetOutboxStatus() == outboxCompleted, nil
	case organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING,
		organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPING,
		organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPED:
		return false, &stuckError{msg: fmt.Sprintf("cluster is %s and will not run", clusterStatusToString(cluster.GetStatus()))}
	default:
		// Provisioning, starting, upgrading, error, unspecified: keep polling.
		return false, nil
	}
}

// waitForClusterDeleted polls ListClusters until Gardener has removed the
// cluster's Shoot. GetCluster cannot tell: it reports a cluster as not found
// as soon as its deletion is requested, while ListClusters keeps listing it
// until Gardener confirms the Shoot is gone.
func waitForClusterDeleted(ctx context.Context, client *FundamentClient, clusterID string) error {
	var last *organizationv1.ListClustersResponse_ClusterSummary

	err := pollWithBackoff(ctx, clusterPollInterval, clusterWaitMaxConsecutiveErrors, func(ctx context.Context) (bool, bool, error) {
		resp, err := client.ClusterService.ListClusters(ctx, organizationv1.ListClustersRequest_builder{}.Build())
		if err != nil {
			return false, false, fmt.Errorf("polling cluster status: %w", err)
		}

		idx := slices.IndexFunc(resp.GetClusters(), func(c *organizationv1.ListClustersResponse_ClusterSummary) bool {
			return c.GetId() == clusterID
		})
		if idx < 0 {
			return true, false, nil
		}

		last = resp.GetClusters()[idx]
		if err := syncGivenUp(last.GetSyncState(), "deleting the cluster"); err != nil {
			return false, true, err
		}
		return false, false, nil
	})

	if err != nil {
		desc := "no status read"
		if last != nil {
			desc = describeClusterState(last.GetStatus(), last.GetSyncState())
		}
		return waitError(ctx, err, "to be deleted", desc)
	}
	return nil
}

// syncGivenUp returns an error once cluster-worker has stopped retrying to
// apply the cluster's latest change to Gardener.
func syncGivenUp(sync *organizationv1.SyncState, action string) error {
	if sync.GetOutboxStatus() != outboxFailed {
		return nil
	}
	return &stuckError{msg: fmt.Sprintf("fundament gave up %s in Gardener: %s", action, sync.GetOutboxError())}
}

func describeClusterState(status organizationv1.ClusterStatus, sync *organizationv1.SyncState) string {
	desc := "last status: " + clusterStatusToString(status)
	if sync.GetShootMessage() != "" {
		desc += ", " + sync.GetShootMessage()
	}
	if sync.GetOutboxStatus() != "" && sync.GetOutboxStatus() != outboxCompleted {
		desc += "; sync " + sync.GetOutboxStatus()
		if sync.GetOutboxError() != "" {
			desc += ": " + sync.GetOutboxError()
		}
	}
	return desc
}
