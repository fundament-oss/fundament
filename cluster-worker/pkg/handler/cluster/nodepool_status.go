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
)

// A node pool row is a desired shape; whether Gardener can stand machines
// under it only shows in the Shoot's EveryNodeReady condition and lastErrors,
// which name the pool's machine deployments ("<shoot>-<pool>-z<zone>").
// These rules turn that into a per-pool status. Pure, like nextShootStatus.

type storedNodePool struct {
	Name    string
	Status  string // tenant.node_pools.status; "" when never classified
	Message string // tenant.node_pools.status_message
}

type nodePoolUpdate struct {
	Name    string
	Status  dbconst.NodePoolStatus
	Message string
	Events  []statusEvent
}

func nextNodePoolStatuses(stored []storedNodePool, observed *gardener.ShootStatus) []nodePoolUpdate {
	var updates []nodePoolUpdate
	for _, pool := range stored {
		next, message, ok := classifyNodePool(pool.Name, observed)
		if !ok || (string(next) == pool.Status && message == pool.Message) {
			continue
		}
		updates = append(updates, nodePoolUpdate{
			Name:    pool.Name,
			Status:  next,
			Message: message,
			Events:  nodePoolEvents(pool, next, message),
		})
	}
	return updates
}

// classifyNodePool decides one pool's status from the shoot-level report.
// ok is false when the report says nothing about pools (no shoot yet, shoot
// deleting, or a failure that does not name this pool).
func classifyNodePool(name string, observed *gardener.ShootStatus) (status dbconst.NodePoolStatus, message string, ok bool) {
	if complaint := poolComplaint(name, observed); complaint != "" {
		if observed.Status == gardener.StatusError && !observed.Retrying {
			return dbconst.NodePoolStatus_Error, complaint, true
		}
		return dbconst.NodePoolStatus_Waiting, complaint, true
	}

	switch observed.Status {
	case gardener.StatusProgressing:
		return dbconst.NodePoolStatus_Progressing, "", true
	case gardener.StatusError:
		if observed.Retrying {
			// Gardener retries by itself; for this pool that is still progress.
			return dbconst.NodePoolStatus_Progressing, "", true
		}
		return "", "", false // failed without naming this pool: not its failure
	case gardener.StatusReady:
		return dbconst.NodePoolStatus_Ready, "", true
	case gardener.StatusPending, gardener.StatusDeleting, gardener.StatusDeleted:
		return "", "", false
	default:
		panic(fmt.Sprintf("unhandled shoot status: %s", observed.Status))
	}
}

// poolComplaint returns the Gardener text naming this pool's machine
// deployment, or "". The "-<pool>-z" fragment anchors the match so a pool
// name that prefixes another ("web", "web2") cannot match the wrong text.
func poolComplaint(name string, observed *gardener.ShootStatus) string {
	fragment := "-" + name + "-z"
	if strings.Contains(observed.EveryNodeReadyMessage, fragment) {
		return observed.EveryNodeReadyMessage
	}
	for _, desc := range observed.LastErrors {
		if strings.Contains(desc, fragment) {
			return desc
		}
	}
	return ""
}

// nodePoolEvents records trouble (once per distinct message) and recovery.
// A pool classified for the first time ("" stored) gets no ready event: on
// an existing fleet that would be an event storm saying nothing happened.
func nodePoolEvents(pool storedNodePool, next dbconst.NodePoolStatus, message string) []statusEvent {
	switch next {
	case dbconst.NodePoolStatus_Waiting:
		if string(next) != pool.Status || message != pool.Message {
			return []statusEvent{{Type: dbconst.ClusterEventEventType_NodepoolWaiting, Message: "Node pool " + pool.Name + ": " + message}}
		}
	case dbconst.NodePoolStatus_Error:
		if string(next) != pool.Status || message != pool.Message {
			return []statusEvent{{Type: dbconst.ClusterEventEventType_NodepoolError, Message: "Node pool " + pool.Name + ": " + message}}
		}
	case dbconst.NodePoolStatus_Ready:
		if pool.Status != "" && pool.Status != string(dbconst.NodePoolStatus_Ready) {
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
// the observed shoot status, and persists changes plus their events. Pool
// status is best-effort alongside the cluster poll: failures log and move on.
func (h *Handler) updateNodePoolStatuses(ctx context.Context, clusterID uuid.UUID, observed *gardener.ShootStatus) {
	rows, err := h.queries.NodePoolListByClusterID(ctx, db.NodePoolListByClusterIDParams{ClusterID: clusterID})
	if err != nil {
		h.logger.Warn("failed to load node pools for status", "cluster_id", clusterID, "error", err)
		return
	}
	if len(rows) == 0 {
		return
	}

	stored := make([]storedNodePool, len(rows))
	for i := range rows {
		stored[i] = storedNodePool{Name: rows[i].Name, Status: rows[i].Status.String, Message: rows[i].StatusMessage.String}
	}

	for _, update := range nextNodePoolStatuses(stored, observed) {
		err := h.queries.NodePoolUpdateStatus(ctx, db.NodePoolUpdateStatusParams{
			ClusterID: clusterID,
			PoolName:  update.Name,
			Status:    pgtype.Text{String: string(update.Status), Valid: true},
			Message:   pgtype.Text{String: update.Message, Valid: update.Message != ""},
		})
		if err != nil {
			h.logger.Warn("failed to update node pool status", "cluster_id", clusterID, "pool", update.Name, "error", err)
			continue
		}
		for _, event := range update.Events {
			if _, err := h.queries.ClusterCreateStatusEvent(ctx, db.ClusterCreateStatusEventParams{
				ClusterID: clusterID,
				EventType: string(event.Type),
				Message:   pgtype.Text{String: event.Message, Valid: true},
			}); err != nil {
				h.logger.Warn("failed to create node pool event", "cluster_id", clusterID, "event_type", event.Type, "error", err)
			}
			if event.Type == dbconst.ClusterEventEventType_NodepoolWaiting || event.Type == dbconst.ClusterEventEventType_NodepoolError {
				h.logger.Warn("node pool machines not provisioning", "cluster_id", clusterID, "pool", update.Name, "message", event.Message)
			}
		}
	}
}
