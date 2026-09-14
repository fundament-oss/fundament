package organization

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	prom "github.com/fundament-oss/fundament/organization-api/pkg/prometheus"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// A node pool row in the database is a desired shape: a name, a machine type and
// an autoscale range. How many machines actually stand under that name, whether
// they are up, and which kubelet they run only exists on the cluster itself, so
// those three come from the same per-shoot Prometheus the metrics panels read
// (ADR-0026). A cluster whose monitoring stack is unreachable degrades the way
// the panels do: unknown status and no node count, never an error that empties
// the page.

const (
	// Gardener labels every node with the worker pool it was created for.
	// kube-state-metrics exposes node labels on kube_node_labels, prefixed with
	// `label_` and with every non-alphanumeric character turned into `_`.
	nodePoolNodeLabel = "label_worker_gardener_cloud_pool"

	queryNodePoolMembership = `kube_node_labels`
	queryNodeReady          = `kube_node_status_condition{condition="Ready",status="true"}`
	queryNodeInfo           = `kube_node_info`
)

// nodePoolRuntime is what the cluster says about one pool right now. The zero
// value is what an unreachable Prometheus yields, and reads as "unknown".
type nodePoolRuntime struct {
	nodes int32
	ready int32
	// version is the kubelet version the pool runs, or empty while its nodes
	// disagree (mid-upgrade) — naming one of two versions would be a guess.
	version string
}

func (r nodePoolRuntime) status() organizationv1.NodePoolStatus {
	switch {
	case r.nodes == 0:
		return organizationv1.NodePoolStatus_NODE_POOL_STATUS_UNSPECIFIED
	case r.ready == r.nodes:
		return organizationv1.NodePoolStatus_NODE_POOL_STATUS_HEALTHY
	case r.ready == 0:
		return organizationv1.NodePoolStatus_NODE_POOL_STATUS_UNHEALTHY
	default:
		return organizationv1.NodePoolStatus_NODE_POOL_STATUS_DEGRADED
	}
}

// nodePoolRuntimes reads the live state of every pool on a cluster, keyed by
// pool name. A cluster with no reachable Prometheus returns an empty map, so
// every pool falls back to the zero value.
func (s *Server) nodePoolRuntimes(ctx context.Context, clusterID uuid.UUID, pools int) map[string]nodePoolRuntime {
	// A cluster without pools has nothing to enrich, and asking anyway costs
	// three round trips to the seed ingress.
	if pools == 0 {
		return nil
	}

	client, err := s.promClientFor(ctx, clusterID)
	if err != nil {
		s.logPromUnavailable(ctx, clusterID, err)
		return nil
	}

	now := time.Now()

	var membership, ready, info []prom.Sample

	g, gctx := errgroup.WithContext(ctx)
	query := func(dst *[]prom.Sample, promQL string) {
		g.Go(func() error {
			samples, err := client.Query(gctx, promQL, now)
			if err != nil {
				return fmt.Errorf("query %s: %w", promQL, err)
			}
			*dst = samples
			return nil
		})
	}

	query(&membership, queryNodePoolMembership)
	query(&ready, queryNodeReady)
	query(&info, queryNodeInfo)

	if err := g.Wait(); err != nil {
		s.logger.WarnContext(ctx, "node pool state queries failed", "cluster_id", clusterID, "error", err)
		return nil
	}

	return nodePoolRuntimesFromSamples(membership, ready, info)
}

// nodePoolRuntimesFromSamples folds three per-node sample sets into one entry
// per pool. Membership decides which nodes count: a node kube-state-metrics
// reports without a pool label belongs to no pool we know of and is left out
// rather than guessed at.
func nodePoolRuntimesFromSamples(membership, ready, info []prom.Sample) map[string]nodePoolRuntime {
	readyNodes := make(map[string]bool, len(ready))
	for _, sample := range ready {
		readyNodes[sample.Labels["node"]] = sample.Value == 1
	}

	versions := make(map[string]string, len(info))
	for _, sample := range info {
		versions[sample.Labels["node"]] = sample.Labels["kubelet_version"]
	}

	// A pool's version is only reported once every node agrees on it, so the
	// empty string doubles as "mixed" and hides the row.
	poolVersions := make(map[string]string, len(membership))
	mixed := make(map[string]bool, len(membership))

	runtimes := make(map[string]nodePoolRuntime, len(membership))
	for _, sample := range membership {
		pool := sample.Labels[nodePoolNodeLabel]
		if pool == "" {
			continue
		}
		node := sample.Labels["node"]

		runtime := runtimes[pool]
		runtime.nodes++
		if readyNodes[node] {
			runtime.ready++
		}
		runtimes[pool] = runtime

		version := versions[node]
		if seen, ok := poolVersions[pool]; ok && seen != version {
			mixed[pool] = true
		}
		poolVersions[pool] = version
	}

	for pool, runtime := range runtimes {
		if !mixed[pool] {
			runtime.version = poolVersions[pool]
			runtimes[pool] = runtime
		}
	}

	return runtimes
}
