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
func ComputeReplication(requested string, nodeCount int) (replicas int, failureDomain string, message string) {
	nodes := nodeCount
	if nodes < 1 {
		nodes = 1
	}

	if requested == "" || requested == "auto" {
		replicas = min(3, nodes)
	} else {
		want, err := strconv.Atoi(requested)
		if err != nil || want < 1 {
			want = 1
		}
		replicas = want
		if replicas > nodes {
			message = fmt.Sprintf("requested %d, clamped to %d: only %d node(s) contribute disks", want, nodes, nodes)
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
