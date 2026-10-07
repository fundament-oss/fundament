package main

import "fmt"

// ComputeReplication resolves the replica count and CRUSH failure domain for a
// pool. A nil request derives replicas from the number of contributing nodes
// (capped at 3). An explicit request is clamped to the node count so a pool
// never asks for more host-domain replicas than there are nodes to place them
// on. The failure domain is "host" only when the result spans >=2 nodes;
// otherwise "osd", so single-node clusters still provision.
//
// nodeCount is cluster-wide, not per-pool: a CephBlockPool has no CRUSH rule
// confining it to one pool's disks. See the call sites in the BlockStorage and
// FileStorage reconcilers.
func ComputeReplication(requested *int32, nodeCount int) (replicas int, failureDomain, message string) {
	nodes := max(nodeCount, 1)

	switch {
	case requested == nil:
		replicas = min(3, nodes)
	case *requested < 1:
		// Below the CRD minimum (reachable for Go-constructed objects or
		// during version skew): auto beats honoring it, which would mean
		// unreplicated storage, and the message keeps the fallback visible.
		message = "unrecognised replication request, using auto"
		replicas = min(3, nodes)
	default:
		replicas = int(*requested)
		if replicas > nodes {
			message = fmt.Sprintf("requested %d, clamped to %d: only %d node(s) in the cluster contribute disks", replicas, nodes, nodes)
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
