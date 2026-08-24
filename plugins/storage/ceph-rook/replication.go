package main

import (
	"fmt"
	"strconv"
)

// ComputeReplication resolves the replica count and CRUSH failure domain for a
// pool. "auto" derives replicas from the number of contributing nodes (capped
// at 3). An explicit request is clamped to the node count so a pool never asks
// for more host-domain replicas than there are nodes to place them on. The
// failure domain is "host" only when the result spans >=2 nodes; otherwise
// "osd", so single-node clusters still provision.
//
// nodeCount is cluster-wide, not per-pool: a CephBlockPool has no CRUSH rule
// confining it to one pool's disks. See the call site in reconcilePool.
func ComputeReplication(requested string, nodeCount int) (replicas int, failureDomain, message string) {
	nodes := nodeCount
	if nodes < 1 {
		nodes = 1
	}

	switch want, err := strconv.Atoi(requested); {
	case requested == "" || requested == "auto":
		replicas = min(3, nodes)
	case err != nil || want < 1:
		// The CRD enum keeps this out of the API today. If it ever widens,
		// falling back to auto beats falling back to 1: a typo would otherwise
		// silently turn a pool into unreplicated storage.
		message = fmt.Sprintf("unrecognised replication %q, using auto", requested)
		replicas = min(3, nodes)
	default:
		replicas = want
		if replicas > nodes {
			message = fmt.Sprintf("requested %d, clamped to %d: only %d node(s) in the cluster contribute disks", want, nodes, nodes)
			replicas = nodes
		}
	}

	if replicas >= 2 && nodeCount >= 2 {
		failureDomain = "host"
	} else {
		failureDomain = "osd"
	}
	return replicas, failureDomain, message
}
