package cluster

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
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
	ID           uuid.UUID
	Name         string
	MachineType  string
	AutoscaleMin int32
	Created      time.Time
	Status       string // tenant.node_pools.status; "" when never classified
	Message      string // tenant.node_pools.status_message
	// Nodes is the node count last read from the shoot, nil when none was;
	// NodesRead is when.
	Nodes     *shoot.PoolNodes
	NodesRead time.Time
}

type nodePoolUpdate struct {
	ID      uuid.UUID
	Name    string
	Status  dbconst.NodePoolStatus
	Message string
	Events  []statusEvent
}

// updating says shoot_updating is set: a change fundament sent reached Gardener
// and rolls out on a ready cluster. syncPending says a change was saved but
// its sync has not (yet) applied it to Gardener (see ClusterGetPushState), so
// what Gardener reports predates the change. Together they gate
// what an operation can explain: only a change of ours explains a
// never-classified pool — without one, a NULL status during an operation is an
// existing pool meeting this feature for the first time mid-reconcile, and
// relabeling it would hide its live metrics.
//
// nodes holds each pool's own node counts by pool name, as the shoot's API
// server lists them, counting only nodes created since the pool was; nil when
// they were not read, which leaves the shoot-wide rules alone.
func nextNodePoolStatuses(stored []storedNodePool, observed *gardener.ShootStatus, updating, syncPending bool, nodes map[string]shoot.PoolNodes) []nodePoolUpdate {
	var updates []nodePoolUpdate
	for _, pool := range stored {
		counts, counted := nodes[pool.Name]
		next, message, ok := classifyNodePool(pool, observed, updating || syncPending, syncPending, counts, counted)
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
// stored waiting/error stays stuck while the worker detail exists:
// EveryNodeReady names only the first failing machine deployment, so a detail
// naming another pool — even on a shoot that turned ready while unhealthy —
// does not mean this one recovered.
//
// A pool's own nodes settle it earlier than the shoot does: a create or
// change that hangs on one pool keeps the shoot from turning ready, so
// without them every pool would wait for the slowest. When no Gardener text
// names the pool and the shoot lists at least autoscale_min of its nodes,
// all Ready, it is ready whatever the other pools do.
func classifyNodePool(pool storedNodePool, observed *gardener.ShootStatus, pushed, syncPending bool, nodes shoot.PoolNodes, counted bool) (status dbconst.NodePoolStatus, message string, ok bool) {
	operationRuns := shootOperationRuns(observed)

	// The cluster's initial create builds every pool, so it explains a
	// never-classified pool even though no outbox change of ours is pending:
	// the create's own outbox row completes before the first check runs.
	creating := creatingShoot(observed)

	// What Gardener reports predates this pool: its change has not left the
	// outbox, so a complaint can only be about a deleted predecessor of the
	// same name, and a ready shoot is the state before the change.
	staleReport := syncPending &&
		(pool.Status == "" || pool.Status == string(dbconst.NodePoolStatus_Progressing))

	if text, own := poolText(pool.Name, observed); text != "" && !staleReport {
		settled := pool.Status == "" || pool.Status == string(dbconst.NodePoolStatus_Ready)
		if settled && !pushed && !creating {
			// A complaint about a settled pool with no change of ours in
			// flight is a node problem (unhealthy, not unprovisioned): its
			// live metrics status describes it better than Waiting would.
			// A NULL status is a pool from before this feature: also settled.
			return "", "", false
		}
		machineType := pool.MachineType
		if syncPending {
			// Gardener's text predates a saved change, which may be this
			// pool's new machine type.
			machineType = ""
		}
		reason := plainReason(machineType, own)
		if reason == "" {
			reason = reasonText(text)
		}
		if observed.Status == gardener.StatusError && !observed.Retrying {
			return dbconst.NodePoolStatus_Error, reason, true
		}
		return dbconst.NodePoolStatus_Waiting, reason, true
	}

	if counted && !staleReport && ownNodesStand(pool, nodes) && awaitsMachines(pool.Status, pushed || creating) {
		return dbconst.NodePoolStatus_Ready, "", true
	}

	switch {
	case observed.Status == gardener.StatusReady:
		if staleReport {
			return dbconst.NodePoolStatus_Progressing, "", true
		}
		if pool.Status == "" && pushed {
			// The change reached Gardener but the gardenlet has not started
			// on it: the pool's machines are coming.
			return dbconst.NodePoolStatus_Progressing, "", true
		}
		if poolStuck(pool.Status) && hasWorkerDetail(observed) {
			// The shoot turned ready while still unhealthy and the detail
			// names another pool's failure: that says nothing about this one.
			return "", "", false
		}
		return dbconst.NodePoolStatus_Ready, "", true
	case operationRuns && pool.Status == "" && (pushed || creating):
		return dbconst.NodePoolStatus_Progressing, "", true
	default:
		return "", "", false
	}
}

func shootOperationRuns(observed *gardener.ShootStatus) bool {
	return observed.Status == gardener.StatusProgressing ||
		(observed.Status == gardener.StatusError && observed.Retrying)
}

func creatingShoot(observed *gardener.ShootStatus) bool {
	return shootOperationRuns(observed) && observed.Operation == gardener.OperationCreate
}

// ownNodesStand reports whether the shoot lists the pool's minimum of nodes,
// all of them Ready. A pool scaled to zero has no nodes to prove anything.
func ownNodesStand(pool storedNodePool, nodes shoot.PoolNodes) bool {
	return nodes.Nodes > 0 && nodes.Nodes >= pool.AutoscaleMin && nodes.Ready == nodes.Nodes
}

// awaitsMachines reports whether a pool is still waiting for its machines:
// progressing or waiting, or never classified while a change of ours or the
// initial create shapes it. An error is final until a new change, and a
// never-classified pool without one is an existing pool, which its live
// metrics describe.
func awaitsMachines(status string, shaped bool) bool {
	switch status {
	case string(dbconst.NodePoolStatus_Progressing), string(dbconst.NodePoolStatus_Waiting):
		return true
	case "":
		return shaped
	default:
		return false
	}
}

func poolStuck(status string) bool {
	return status == string(dbconst.NodePoolStatus_Waiting) || status == string(dbconst.NodePoolStatus_Error)
}

func hasWorkerDetail(observed *gardener.ShootStatus) bool {
	return observed.EveryNodeReadyMessage != "" || len(observed.LastErrors) > 0
}

// poolComplaint returns the cleaned Gardener text naming this pool's machine
// deployment, or "" (see poolText).
func poolComplaint(name string, observed *gardener.ShootStatus) string {
	text, _ := poolText(name, observed)
	return reasonText(text)
}

// poolText returns the raw Gardener text naming this pool's machine
// deployment, or "", and own: the part of it after the pool's name, up to the
// next machine name of the shoot, which is about this pool's machines alone.
// Gardener's providers name deployments, machines and nodes
// <technicalID>-<worker>, where <worker> is the generated worker name
// (kubename.GenerateWorkerName), never the raw pool name. What follows
// differs per provider: -z<n> on the local provider, the machine set's hash
// on metal-stack. So the match is the whole <technicalID>-<worker> ending at
// a name boundary: a project or shoot named like the worker, another shoot's
// machines and another pool's worker name all fail it. Without a technical
// ID Gardener has created no machines, and nothing names a pool.
func poolText(name string, observed *gardener.ShootStatus) (text, own string) {
	if !hasWorkerDetail(observed) || observed.TechnicalID == "" {
		return "", ""
	}
	workerPrefix := observed.TechnicalID + "-" + kubename.GenerateWorkerName(name)
	for _, candidate := range append([]string{observed.EveryNodeReadyMessage}, observed.LastErrors...) {
		rest, found := afterWorker(candidate, workerPrefix)
		if !found {
			continue
		}
		if next := strings.Index(rest, observed.TechnicalID+"-"); next >= 0 {
			rest = rest[:next]
		}
		return candidate, rest
	}
	return "", ""
}

// afterWorker returns the text after the first occurrence of workerPrefix as
// a whole name part: followed by the end of the text or by a character no
// Kubernetes name continues a part with (anything but a lowercase letter or a
// digit).
func afterWorker(text, workerPrefix string) (rest string, found bool) {
	for {
		_, rest, found = strings.Cut(text, workerPrefix)
		if !found {
			return "", false
		}
		if rest == "" || !isNameChar(rest[0]) {
			return rest, true
		}
		text = rest
	}
}

func isNameChar(c byte) bool {
	return ('a' <= c && c <= 'z') || ('0' <= c && c <= '9')
}

// maxReasonLen caps Gardener's text before it is stored and shown: it is node
// and provider text, unbounded, and may carry terminal control codes.
const maxReasonLen = 1024

// reasonText turns line breaks and tabs into spaces, drops other control
// characters and caps the length.
func reasonText(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return ' '
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0):
			return -1
		default:
			return r
		}
	}, s)
	if runes := []rune(s); len(runes) > maxReasonLen {
		s = string(runes[:maxReasonLen-1]) + "…"
	}
	return s
}

// knownReasons turn provider text that Gardener passes on into words a user
// understands. Gardener has an error code for a depleted resource, but
// provider-metal sets no error codes, so its text is all there is to go by.
var knownReasons = []struct {
	fragment string
	reason   func(machineType string) string
}{
	// provider-metal: metal-api has no free machine of the pool's size.
	{`no machine available`, func(machineType string) string {
		if machineType == "" {
			return "No free machine is available for this pool."
		}
		return "No free " + machineType + " machine is available."
	}},
}

// plainReason returns a known provider failure in the part of Gardener's
// text about one pool's machines in plain words, or "". It reads the raw
// text, so the cap on stored text cannot cut the failure off. machineType is
// "" when it may not be the type Gardener's text is about. The text itself is
// logged by updateNodePoolStatuses.
func plainReason(machineType, own string) string {
	for _, known := range knownReasons {
		if strings.Contains(own, known.fragment) {
			return known.reason(machineType)
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
// log and move on; the next check retries. It returns how soon to check
// again while the pools' nodes are read, or 0.
func (h *Handler) updateNodePoolStatuses(ctx context.Context, cluster *db.ClusterGetForStatusCheckRow, observed *gardener.ShootStatus) time.Duration {
	switch observed.Status {
	case gardener.StatusPending, gardener.StatusDeleting, gardener.StatusDeleted, gardener.StatusLost:
		return 0 // nothing a pool could learn from these
	case gardener.StatusProgressing, gardener.StatusReady, gardener.StatusError:
	default:
		panic(fmt.Sprintf("unhandled shoot status: %s", observed.Status))
	}

	rows, err := h.queries.NodePoolListStatusByClusterID(ctx, db.NodePoolListStatusByClusterIDParams{ClusterID: cluster.ID})
	if err != nil {
		h.logger.Warn("failed to load node pools for status", "cluster_id", cluster.ID, "error", err)
		return 0
	}
	if len(rows) == 0 {
		return 0
	}

	stored := make([]storedNodePool, len(rows))
	for i := range rows {
		stored[i] = storedNodePool{
			ID: rows[i].ID, Name: rows[i].Name, MachineType: rows[i].MachineType, AutoscaleMin: rows[i].AutoscaleMin,
			Created: rows[i].Created.Time, Status: rows[i].Status.String, Message: rows[i].StatusMessage.String,
		}
		if rows[i].NodeCount.Valid && rows[i].NodeCountReady.Valid && rows[i].NodeCountUpdated.Valid {
			stored[i].Nodes = &shoot.PoolNodes{Nodes: rows[i].NodeCount.Int32, Ready: rows[i].NodeCountReady.Int32}
			stored[i].NodesRead = rows[i].NodeCountUpdated.Time
		}
	}

	// Read what is in flight after the pool list, not from the cluster row the
	// check started with: a pool inserted since then commits together with its
	// outbox row, and classifying it from the stale state would call it ready.
	push, err := h.queries.ClusterGetPushState(ctx, db.ClusterGetPushStateParams{ClusterID: cluster.ID})
	if err != nil {
		h.logger.Warn("failed to load cluster push state for node pool status", "cluster_id", cluster.ID, "error", err)
		return 0
	}
	var (
		counts  map[string]poolNodeCounts
		recheck time.Duration
	)
	if h.poolNodes != nil && readsPoolNodes(stored, observed, push.ShootUpdating || push.SyncPending) {
		counts = h.poolNodesFor(ctx, cluster.ID, stored)
		recheck = poolNodeRecheck
	}
	updated := make(map[uuid.UUID]bool, len(stored))
	for _, update := range nextNodePoolStatuses(stored, observed, push.ShootUpdating, push.SyncPending, ownNodeCounts(counts)) {
		var poolCounts *shoot.PoolNodes
		if c, ok := counts[update.Name]; ok {
			poolCounts = &c.all
		}
		updated[update.ID] = true
		if err := h.writeNodePoolStatus(ctx, cluster.ID, update, poolCounts); err != nil {
			h.logger.Warn("failed to update node pool status", "cluster_id", cluster.ID, "pool", update.Name, "error", err)
			continue
		}
		for _, event := range update.Events {
			if event.Type == dbconst.ClusterEventEventType_NodepoolWaiting || event.Type == dbconst.ClusterEventEventType_NodepoolError {
				h.logger.Warn("node pool machines not provisioning", "cluster_id", cluster.ID, "pool", update.Name, "message", update.Message,
					"gardener_text", poolComplaint(update.Name, observed))
			}
		}
	}

	// Pools whose status stays still get the counts just read, when they
	// changed or the stored ones are getting old.
	now := time.Now()
	for _, pool := range stored {
		c, ok := counts[pool.Name]
		if !ok || updated[pool.ID] || !nodeCountChanged(pool, c.all, now) {
			continue
		}
		if err := h.writeNodePoolCount(ctx, pool.ID, c.all); err != nil {
			h.logger.Warn("failed to record node pool node count", "cluster_id", cluster.ID, "pool", pool.Name, "error", err)
		}
	}
	return recheck
}

// writeNodePoolStatus commits one pool's status row, its events and, when
// counts is set, the node counts the status was decided on, together.
func (h *Handler) writeNodePoolStatus(ctx context.Context, clusterID uuid.UUID, update nodePoolUpdate, counts *shoot.PoolNodes) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	qtx := h.queries.WithTx(tx)
	updated, err := qtx.NodePoolUpdateStatus(ctx, db.NodePoolUpdateStatusParams{
		NodePoolID: update.ID,
		Status:     pgtype.Text{String: string(update.Status), Valid: true},
		Message:    pgtype.Text{String: update.Message, Valid: update.Message != ""},
	})
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	if updated == 0 {
		// The pool was soft-deleted after it was read: no row, no events.
		return nil
	}
	if counts != nil {
		if _, err := qtx.NodePoolUpdateNodeCount(ctx, nodeCountParams(update.ID, *counts)); err != nil {
			return fmt.Errorf("update node count: %w", err)
		}
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
