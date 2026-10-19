package cluster

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/common/dbconst"
	"github.com/fundament-oss/fundament/common/kubename"
)

// A node pool row is a desired shape; whether Gardener can stand machines
// under it only shows in the Shoot's EveryNodeReady condition and lastErrors,
// which name the pool's machine deployments. Those names carry the generated
// worker name (kubename.GenerateWorkerName), not the pool name the user chose.
// These rules turn that into a per-pool status. Pure, like nextShootStatus.

type storedNodePool struct {
	ID      uuid.UUID
	Name    string
	Status  string // tenant.node_pools.status; "" when never classified
	Message string // tenant.node_pools.status_message
}

type nodePoolUpdate struct {
	ID      uuid.UUID
	Name    string
	Status  dbconst.NodePoolStatus
	Message string
	Events  []statusEvent
}

// pushed says a change fundament sent is in flight for this cluster (a
// pending outbox row, or shoot_updating while Gardener rolls it out). Only
// then does an operation explain a never-classified pool: it is the change
// that creates its machines. Without it, a NULL status during an operation is
// an existing pool meeting this feature for the first time mid-reconcile, and
// relabeling it would hide its live metrics.
func nextNodePoolStatuses(stored []storedNodePool, observed *gardener.ShootStatus, pushed bool) []nodePoolUpdate {
	var updates []nodePoolUpdate
	for _, pool := range stored {
		next, message, ok := classifyNodePool(pool, observed, pushed)
		if !ok || (string(next) == pool.Status && message == pool.Message) {
			continue
		}
		updates = append(updates, nodePoolUpdate{
			ID:      pool.ID,
			Name:    pool.Name,
			Status:  next,
			Message: message,
			Events:  nodePoolEvents(pool, next, message),
		})
	}
	return updates
}

// classifyNodePool decides one pool's status from the shoot-level report.
// ok is false when the report says nothing about this pool, which keeps the
// stored value: no shoot yet, shoot deleting, a failure that does not name
// the pool, or an operation in flight that does not name it either — a
// routine reconcile says nothing about pools whose machines stand. A pool
// stored waiting/error that the current worker detail no longer names has
// been dealt with: its complaint moved on, so it returns to progressing
// until the shoot settles.
func classifyNodePool(pool storedNodePool, observed *gardener.ShootStatus, pushed bool) (status dbconst.NodePoolStatus, message string, ok bool) {
	if complaint := poolComplaint(pool.Name, observed); complaint != "" {
		if observed.Status == gardener.StatusError && !observed.Retrying {
			return dbconst.NodePoolStatus_Error, complaint, true
		}
		return dbconst.NodePoolStatus_Waiting, complaint, true
	}

	operationRuns := observed.Status == gardener.StatusProgressing ||
		(observed.Status == gardener.StatusError && observed.Retrying)

	switch {
	case observed.Status == gardener.StatusReady:
		if pool.Status == "" && pushed {
			// The pool was just saved and its sync has not reached Gardener
			// yet (or the gardenlet has not started): its machines are coming.
			return dbconst.NodePoolStatus_Progressing, "", true
		}
		return dbconst.NodePoolStatus_Ready, "", true
	case operationRuns && pool.Status == "" && pushed:
		return dbconst.NodePoolStatus_Progressing, "", true
	case operationRuns && poolStuck(pool.Status) && hasWorkerDetail(observed):
		// The detail names other pools now; this one's complaint is gone.
		return dbconst.NodePoolStatus_Progressing, "", true
	default:
		return "", "", false
	}
}

func poolStuck(status string) bool {
	return status == string(dbconst.NodePoolStatus_Waiting) || status == string(dbconst.NodePoolStatus_Error)
}

func hasWorkerDetail(observed *gardener.ShootStatus) bool {
	return observed.EveryNodeReadyMessage != "" || len(observed.LastErrors) > 0
}

// poolComplaint returns the Gardener text naming this pool's machine
// deployment, or "". Gardener names machine deployments after the generated
// worker name — a fixed-length prefix+hash (kubename.GenerateWorkerName) — so
// the match looks for that name, never the raw pool name, and two pools can
// never match each other's text: their generated names have equal length and
// distinct hash suffixes, so neither can contain the other.
func poolComplaint(name string, observed *gardener.ShootStatus) string {
	if !hasWorkerDetail(observed) {
		return ""
	}
	workerName := kubename.GenerateWorkerName(name)
	if strings.Contains(observed.EveryNodeReadyMessage, workerName) {
		return observed.EveryNodeReadyMessage
	}
	for _, desc := range observed.LastErrors {
		if strings.Contains(desc, workerName) {
			return desc
		}
	}
	return ""
}

// nodePoolEvents records transitions only: into waiting/error, and into
// ready from a non-ready status. Gardener rewrites its reason text as
// machines and counts churn, so a message-only change refreshes the row
// silently — the row carries the current reason, the timeline the turning
// points. A pool classified for the first time ("" stored) gets no ready
// event: on an existing fleet that would be an event storm saying nothing
// happened.
func nodePoolEvents(pool storedNodePool, next dbconst.NodePoolStatus, message string) []statusEvent {
	if string(next) == pool.Status {
		return nil
	}
	switch next {
	case dbconst.NodePoolStatus_Waiting:
		return []statusEvent{{Type: dbconst.ClusterEventEventType_NodepoolWaiting, Message: "Node pool " + pool.Name + ": " + message}}
	case dbconst.NodePoolStatus_Error:
		return []statusEvent{{Type: dbconst.ClusterEventEventType_NodepoolError, Message: "Node pool " + pool.Name + ": " + message}}
	case dbconst.NodePoolStatus_Ready:
		if pool.Status != "" {
			return []statusEvent{{Type: dbconst.ClusterEventEventType_NodepoolReady, Message: "Node pool " + pool.Name + ": machines are ready"}}
		}
	case dbconst.NodePoolStatus_Progressing:
		// No event: the sync_succeeded event already said a change started.
	default:
		panic(fmt.Sprintf("unhandled node pool status: %s", next))
	}
	return nil
}

// updateNodePoolStatuses loads the cluster's pools, classifies them against
// the observed shoot status, and persists changes plus their events — each
// pool's row and events in one transaction, so a crash cannot keep a status
// whose event was lost. Best-effort alongside the cluster check: failures
// log and move on; the next check retries.
func (h *Handler) updateNodePoolStatuses(ctx context.Context, cluster *db.ClusterGetForStatusCheckRow, observed *gardener.ShootStatus) {
	switch observed.Status {
	case gardener.StatusPending, gardener.StatusDeleting, gardener.StatusDeleted:
		return // nothing a pool could learn from these
	case gardener.StatusProgressing, gardener.StatusReady, gardener.StatusError:
	default:
		panic(fmt.Sprintf("unhandled shoot status: %s", observed.Status))
	}

	rows, err := h.queries.NodePoolListStatusByClusterID(ctx, db.NodePoolListStatusByClusterIDParams{ClusterID: cluster.ID})
	if err != nil {
		h.logger.Warn("failed to load node pools for status", "cluster_id", cluster.ID, "error", err)
		return
	}
	if len(rows) == 0 {
		return
	}

	stored := make([]storedNodePool, len(rows))
	for i := range rows {
		stored[i] = storedNodePool{ID: rows[i].ID, Name: rows[i].Name, Status: rows[i].Status.String, Message: rows[i].StatusMessage.String}
	}

	pushed := cluster.ShootUpdating ||
		(cluster.OutboxStatus.Valid && cluster.OutboxStatus.String != string(dbconst.ClusterOutboxStatus_Completed))

	for _, update := range nextNodePoolStatuses(stored, observed, pushed) {
		if err := h.writeNodePoolStatus(ctx, cluster.ID, update); err != nil {
			h.logger.Warn("failed to update node pool status", "cluster_id", cluster.ID, "pool", update.Name, "error", err)
			continue
		}
		for _, event := range update.Events {
			if event.Type == dbconst.ClusterEventEventType_NodepoolWaiting || event.Type == dbconst.ClusterEventEventType_NodepoolError {
				h.logger.Warn("node pool machines not provisioning", "cluster_id", cluster.ID, "pool", update.Name, "message", update.Message)
			}
		}
	}
}

// writeNodePoolStatus commits one pool's status row and its events together.
func (h *Handler) writeNodePoolStatus(ctx context.Context, clusterID uuid.UUID, update nodePoolUpdate) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	qtx := h.queries.WithTx(tx)
	if err := qtx.NodePoolUpdateStatus(ctx, db.NodePoolUpdateStatusParams{
		NodePoolID: update.ID,
		Status:     pgtype.Text{String: string(update.Status), Valid: true},
		Message:    pgtype.Text{String: update.Message, Valid: update.Message != ""},
	}); err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	for _, event := range update.Events {
		if _, err := qtx.ClusterCreateStatusEvent(ctx, db.ClusterCreateStatusEventParams{
			ClusterID: clusterID,
			EventType: string(event.Type),
			Message:   pgtype.Text{String: event.Message, Valid: true},
		}); err != nil {
			return fmt.Errorf("create %s event: %w", event.Type, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
