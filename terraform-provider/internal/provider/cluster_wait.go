package provider

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"

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

// waitForClusterRunning polls GetCluster until cluster-worker has applied
// every change to Gardener, Gardener has finished rolling it out
// (shoot_updating cleared), and the cluster is usable. The wait does not hold
// out for health: a cluster that serves while its health checks settle
// (starting) or while a health condition fails with no rollout running
// (unhealthy, such as a node pool that cannot get machines) ends the wait too
// — holding every apply hostage to a degraded node would never return.
// Callers warn when the wait ends on one of those.
//
// changed says whether the caller sent cluster-worker a change. Only a change
// creates a lost cluster again, so without one a lost cluster ends the wait at
// once instead of at the deadline.
//
// A failed sync is fatal: cluster-worker has stopped retrying it. An error
// status is not: Gardener retries most errors, so it only ends the wait when
// the deadline passes, with Gardener's message in the error. It returns the
// cluster as last read, so callers need not read it again.
func waitForClusterRunning(ctx context.Context, client *FundamentClient, clusterID string, changed bool) (*organizationv1.ClusterDetails, error) {
	var last *organizationv1.ClusterDetails

	err := pollWithBackoff(ctx, clusterPollInterval, clusterWaitMaxConsecutiveErrors, func(ctx context.Context) (bool, bool, error) {
		resp, err := client.ClusterService.GetCluster(ctx, organizationv1.GetClusterRequest_builder{
			ClusterId: clusterID,
		}.Build())
		if err != nil {
			return false, false, fmt.Errorf("polling cluster status: %w", err)
		}

		last = resp.GetCluster()
		done, err := clusterRunning(last, changed)
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
// should: Fundament gave up its sync, the cluster is being deleted, or it is
// lost with no change to create it again.
type stuckError struct{ msg string }

func (e *stuckError) Error() string { return e.msg }

// waitInconclusive reports whether a wait ended without learning that the
// cluster is stuck: it timed out, was interrupted, or lost the API. Fundament
// then carries on with the cluster.
func waitInconclusive(err error) bool {
	var stuck *stuckError
	return !errors.As(err, &stuck)
}

// clusterNotSettledWarning is the warning when a wait ends while the cluster
// serves but is not plainly running, or "" when there is nothing to say.
func clusterNotSettledWarning(cluster *organizationv1.ClusterDetails) string {
	switch cluster.GetStatus() {
	case organizationv1.ClusterStatus_CLUSTER_STATUS_STARTING:
		return "The cluster serves, but its health checks have not passed yet. Fundament keeps watching it; `tofu plan` shows its status."
	case organizationv1.ClusterStatus_CLUSTER_STATUS_UNHEALTHY:
		return "The cluster serves, but a health condition is failing — for example a node pool that cannot get machines. " +
			"Gardener keeps working on it; the cluster's activity log and `tofu plan` show its status."
	default:
		return ""
	}
}

// clusterRunning reports whether the cluster runs with all changes applied, or
// an error once it cannot get there. changed is as for waitForClusterRunning.
func clusterRunning(cluster *organizationv1.ClusterDetails, changed bool) (bool, error) {
	sync := cluster.GetSyncState()
	if err := syncGivenUp(sync, "applying the cluster"); err != nil {
		return false, err
	}

	switch cluster.GetStatus() {
	case organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING,
		organizationv1.ClusterStatus_CLUSTER_STATUS_STARTING,
		organizationv1.ClusterStatus_CLUSTER_STATUS_UNHEALTHY:
		// Starting (health checks settling after create) and unhealthy (health
		// conditions failing, such as machines a node pool cannot get) are both
		// ready in Gardener: the cluster is usable, and the wait must not hold
		// every later apply hostage to a degraded node. But while Gardener
		// still rolls out a change of ours (shoot_updating), the picture is
		// not final: returning then would hand downstream resources a
		// half-rolled-out cluster, so the wait continues until the rollout
		// ends — or the timeout passes, with the status in the error.
		// The outbox gate: the status is Gardener's last reading, and a change
		// still in the outbox has not reached it.
		done := !sync.HasOutboxStatus() || sync.GetOutboxStatus() == outboxCompleted
		return done && !sync.GetShootUpdating(), nil
	case organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING,
		organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPING,
		organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPED:
		return false, &stuckError{msg: fmt.Sprintf("cluster is %s and will not run", clusterStatusToString(cluster.GetStatus()))}
	case organizationv1.ClusterStatus_CLUSTER_STATUS_LOST:
		// The caller's change creates the cluster again; the status check
		// moves it out of lost once Gardener has the new Shoot. Without a
		// change, as when only a plugin is installed, nothing will. The
		// cluster's sync state cannot tell: a node pool change syncs the
		// cluster through the node pool's own outbox row.
		if changed {
			return false, nil
		}
		return false, &stuckError{msg: "cluster is lost: it no longer exists in Gardener, and only a change to the cluster or its node pools creates it again, empty"}
	default:
		// Provisioning, upgrading, error, unspecified: keep polling.
		return false, nil
	}
}

// waitForClusterDeleted polls GetCluster until Gardener has removed the
// cluster's Shoot: GetCluster returns a deleted cluster as deleting until then,
// and reports it as not found after. Where GetCluster refuses, listedCluster
// tells whether it is still there.
func waitForClusterDeleted(ctx context.Context, client *FundamentClient, clusterID string) error {
	desc := "no status read"

	err := pollWithBackoff(ctx, clusterPollInterval, clusterWaitMaxConsecutiveErrors, func(ctx context.Context) (bool, bool, error) {
		resp, err := client.ClusterService.GetCluster(ctx, organizationv1.GetClusterRequest_builder{
			ClusterId: clusterID,
		}.Build())
		if err != nil {
			code := connect.CodeOf(err)
			if code != connect.CodeNotFound && code != connect.CodePermissionDenied {
				return false, false, fmt.Errorf("polling cluster status: %w", err)
			}
			listed, listErr := listedCluster(ctx, client, clusterID)
			if listErr != nil {
				return false, false, listErr
			}
			if listed == nil {
				return true, false, nil
			}
			if code == connect.CodePermissionDenied && listed.GetStatus() != organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING {
				return false, true, fmt.Errorf("polling cluster status: %w", err)
			}
			desc = describeClusterState(listed.GetStatus(), listed.GetSyncState())
			if err := syncGivenUp(listed.GetSyncState(), "deleting the cluster"); err != nil {
				return false, true, err
			}
			return false, false, nil
		}

		cluster := resp.GetCluster()
		desc = describeClusterState(cluster.GetStatus(), cluster.GetSyncState())
		if err := syncGivenUp(cluster.GetSyncState(), "deleting the cluster"); err != nil {
			return false, true, err
		}
		return false, false, nil
	})

	if err != nil {
		return waitError(ctx, err, "to be deleted", desc)
	}
	return nil
}

// listedCluster returns the cluster as ListClusters lists it, or nil when it
// is not listed. It backs GetCluster up where GetCluster refuses: an
// organization-api from before fundament#707 refuses a cluster as soon as its
// deletion is requested, while ListClusters keeps listing it until Gardener
// has removed it. The provider is released apart from the servers it talks
// to; drop this once none of them predates that change.
func listedCluster(ctx context.Context, client *FundamentClient, clusterID string) (*organizationv1.ListClustersResponse_ClusterSummary, error) {
	listResp, err := client.ClusterService.ListClusters(ctx, organizationv1.ListClustersRequest_builder{}.Build())
	if err != nil {
		return nil, fmt.Errorf("unable to list clusters: %w", err)
	}
	for _, c := range listResp.GetClusters() {
		if c.GetId() == clusterID {
			return c, nil
		}
	}
	return nil, nil
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
