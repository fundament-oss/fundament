package shoot

import (
	"context"
	"fmt"
	"time"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PoolNodes is how many of a worker pool's nodes there are, and how many of
// them are Ready.
type PoolNodes struct {
	Nodes int32
	Ready int32
}

// PoolNode is one node carrying a worker pool's worker.gardener.cloud/pool
// label, as the shoot's API server lists it.
type PoolNode struct {
	Created time.Time
	Ready   bool
}

// CountPoolNodes counts the nodes created at or after since; a zero since
// counts them all. A pool replaced by one of the same name has the same
// worker name, so its predecessor's nodes carry the same label: only nodes
// created after the pool was are its own.
func CountPoolNodes(nodes []PoolNode, since time.Time) PoolNodes {
	var counts PoolNodes
	for _, node := range nodes {
		if node.Created.Before(since) {
			continue
		}
		counts.Nodes++
		if node.Ready {
			counts.Ready++
		}
	}
	return counts
}

// nodeCredentialTTL is how long ListNodePools reuses a shoot's admin
// credentials. They are requested for 600 seconds, so a reused set always has
// minutes left; reading nodes once a minute while a pool waits for machines
// then costs one set per five minutes instead of one per read.
const nodeCredentialTTL = 5 * time.Minute

func (r *RealShootAccess) ListNodePools(ctx context.Context, clusterID uuid.UUID) (map[string][]PoolNode, error) {
	if r.nodeCredentials != nil && !HasClientCache(ctx) {
		ctx = withClientCache(ctx, r.nodeCredentials)
	}
	cs, err := r.newClient(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	// resourceVersion=0 lets the API server answer from its watch cache: the
	// status check asks often while a pool is coming up, and a list that is
	// a moment old is good enough.
	list, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		LabelSelector:   v1beta1constants.LabelWorkerPool,
		ResourceVersion: "0",
	})
	if err != nil {
		// The shoot may have been recreated with new credentials: the next
		// read requests a fresh set.
		if r.nodeCredentials != nil {
			r.nodeCredentials.drop(clusterID)
		}
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	return groupPoolNodes(list.Items), nil
}

// groupPoolNodes groups nodes by worker pool label value, which is the
// generated worker name (kubename.GenerateWorkerName), not the pool name.
// A node without the label belongs to no pool and is left out.
func groupPoolNodes(nodes []corev1.Node) map[string][]PoolNode {
	pools := make(map[string][]PoolNode)
	for i := range nodes {
		pool, ok := nodes[i].Labels[v1beta1constants.LabelWorkerPool]
		if !ok || pool == "" {
			continue
		}
		pools[pool] = append(pools[pool], PoolNode{
			Created: nodes[i].CreationTimestamp.Time,
			Ready:   nodeReady(&nodes[i]),
		})
	}
	return pools
}

// nodeReady reports the node's Ready condition; a node that reports none is
// not ready.
func nodeReady(node *corev1.Node) bool {
	for _, c := range node.Status.Conditions {
		if c.Type == corev1.NodeReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}
