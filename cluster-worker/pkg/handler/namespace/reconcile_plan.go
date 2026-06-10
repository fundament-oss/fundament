package namespace

import (
	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
)

// Plan is the set of actions reconcile derives from comparing the DB's desired
// set of namespaces against what exists on the shoot cluster.
type Plan struct {
	// CreateIDs are namespace ids that should exist on the cluster but don't.
	// Reconcile enqueues an outbox row for each; the Sync handler does the work.
	CreateIDs []uuid.UUID
	// DeleteNames are cluster-side namespace names that carry our label but whose
	// id is no longer in the active DB set (row hard-deleted or soft-deleted).
	DeleteNames []string
}

// BuildPlan diffs the desired active namespace ids (from the DB) against the
// fundament-labelled namespaces present on the shoot. It is pure so it can be
// unit-tested without a database or a cluster.
//
//   - active id missing on cluster       -> create (enqueue sync)
//   - cluster ns id not in active set    -> orphan (delete)
//   - cluster ns without our id label    -> ignored. ListNamespaces selects by
//     the label key, so untagged namespaces never reach this function; the
//     guard below is a defensive double-check.
func BuildPlan(activeIDs []uuid.UUID, clusterNamespaces []shoot.ResourceInfo) Plan {
	active := make(map[uuid.UUID]struct{}, len(activeIDs))
	for _, id := range activeIDs {
		active[id] = struct{}{}
	}

	onCluster := make(map[uuid.UUID]struct{}, len(clusterNamespaces))
	var deleteNames []string
	for i := range clusterNamespaces {
		raw, ok := clusterNamespaces[i].Labels[LabelNamespaceID]
		if !ok {
			continue // not ours; never touch
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			continue // malformed label; leave it alone
		}
		if _, isActive := active[id]; !isActive {
			deleteNames = append(deleteNames, clusterNamespaces[i].Name)
			continue
		}
		onCluster[id] = struct{}{}
	}

	var createIDs []uuid.UUID
	for _, id := range activeIDs {
		if _, exists := onCluster[id]; !exists {
			createIDs = append(createIDs, id)
		}
	}

	return Plan{CreateIDs: createIDs, DeleteNames: deleteNames}
}
