package cluster

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/common/dbconst"
)

// Reconcile compares DB state with Gardener state to report lost shoots and
// to finish deleting the shoots of deleted clusters. What it sees is recorded
// for the staff fleet view (FUN-23): the shoots, the disagreements as drift
// findings, and that the pass completed.
func (h *Handler) Reconcile(ctx context.Context) error {
	if ctx.Err() != nil {
		return nil //nolint:nilerr // graceful shutdown
	}

	h.logger.Info("starting cluster reconciliation")

	dbClusters, err := h.queries.ClusterListActive(ctx)
	if err != nil {
		h.logger.Error("failed to list clusters from DB", "error", err)
		return fmt.Errorf("list active clusters: %w", err)
	}

	shoots, err := h.gardener.ListShoots(ctx)
	if err != nil {
		h.logger.Error("failed to list shoots from Gardener", "error", err)
		return fmt.Errorf("list shoots: %w", err)
	}

	dbClusterByID := make(map[uuid.UUID]db.ClusterListActiveRow)
	for _, c := range dbClusters {
		dbClusterByID[c.ID] = c
	}

	var findings []driftFinding
	shootByClusterID := make(map[uuid.UUID]gardener.ShootInfo)
	for _, s := range shoots {
		if s.InvalidClusterID {
			findings = append(findings, driftFinding{
				kind:     dbconst.DriftFindingKind_ShootInvalidClusterId,
				shoot:    s,
				observed: s.Labels[gardener.LabelClusterID],
			})
			continue
		}
		// Two shoots with one cluster id (a recreate in flight, a copied
		// label) keep the oldest, so a finding's subject, which includes the
		// shoot uid, does not change with Gardener's list order.
		if prev, ok := shootByClusterID[s.ClusterID]; ok && !olderShoot(s, prev) {
			continue
		}
		shootByClusterID[s.ClusterID] = s
	}

	// Synced clusters whose shoot is missing in Gardener are not recreated: a
	// new, empty shoot under the same name would hide that the workloads and
	// data are gone. The status check records the cluster as lost; a change
	// to the cluster creates the shoot again.
	//
	// The status check only calls a shoot lost once it has seen it, since
	// Gardener can report a just-created shoot as not found. A shoot that
	// disappears before its first check therefore stays pending, and is
	// logged here on every pass until a change to the cluster creates it
	// again.
	var missingCount int
	for _, cluster := range dbClusters {
		if !cluster.LatestSyncCompleted {
			continue
		}
		s, exists := shootByClusterID[cluster.ID]
		if exists {
			if f, ok := versionMismatch(&cluster, s); ok {
				findings = append(findings, f)
			}
			continue
		}
		missingCount++
		findings = append(findings, driftFinding{
			kind:      dbconst.DriftFindingKind_ClusterWithoutShoot,
			clusterID: cluster.ID,
		})
		if gardener.ShootStatusType(cluster.ShootStatus.String) == gardener.StatusLost {
			continue
		}
		h.logger.Error("shoot missing in Gardener, not recreating it",
			"cluster_id", cluster.ID, "name", cluster.Name)
		h.EnqueueStatusCheck(cluster.ID)
	}

	orphans, err := h.findOrphans(ctx, dbClusterByID, shootByClusterID)
	if err != nil {
		return err
	}
	for _, id := range orphans.unknown {
		findings = append(findings, driftFinding{
			kind:      dbconst.DriftFindingKind_ShootWithoutCluster,
			clusterID: id,
			shoot:     shootByClusterID[id],
		})
	}
	for _, id := range orphans.deleted {
		findings = append(findings, driftFinding{
			kind:      dbconst.DriftFindingKind_ShootOfDeletedCluster,
			clusterID: id,
			shoot:     shootByClusterID[id],
		})
	}

	// The record is for the staff fleet view; failing to write it must not
	// stop the orphan cleanup. The failure is not stored anywhere else: the
	// heartbeat is part of the same transaction, so the fleet view sees the
	// reconcile pass go overdue and marks the drift findings as such.
	if err := h.recordObservation(ctx, shoots, findings); err != nil {
		h.logger.Error("failed to record Gardener observation", "error", err)
	}

	h.deleteOrphans(ctx, orphans, shootByClusterID)

	h.logger.Info("cluster reconciliation complete",
		"clusters", len(dbClusters),
		"shoots", len(shoots),
		"shoots_missing", missingCount,
		"unknown_shoots", len(orphans.unknown),
		"drift_findings", len(findings))

	return nil
}

// driftFinding is one disagreement a reconcile pass observed. A finding is
// identified by its kind, cluster id and shoot uid; expected and observed
// carry the values a comparing kind disagrees on.
type driftFinding struct {
	kind      dbconst.DriftFindingKind
	clusterID uuid.UUID // uuid.Nil when the shoot's label is no id
	shoot     gardener.ShootInfo
	expected  string
	observed  string
}

// versionMismatch reports a ready cluster, not updating, whose shoot runs
// another Kubernetes minor version. Only the minor is compared: Gardener's
// maintenance moves patch versions forward on its own, and the database keeps
// the version the user chose. The caller has checked that the cluster's latest
// sync completed, so the shoot's spec reflects the database.
func versionMismatch(cluster *db.ClusterListActiveRow, s gardener.ShootInfo) (driftFinding, bool) {
	if gardener.ShootStatusType(cluster.ShootStatus.String) != gardener.StatusReady || cluster.ShootUpdating {
		return driftFinding{}, false
	}
	if minorVersion(cluster.KubernetesVersion) == minorVersion(s.KubernetesVersion) {
		return driftFinding{}, false
	}
	return driftFinding{
		kind:      dbconst.DriftFindingKind_KubernetesVersionMismatch,
		clusterID: cluster.ID,
		shoot:     s,
		expected:  cluster.KubernetesVersion,
		observed:  s.KubernetesVersion,
	}, true
}

// olderShoot orders shoots by creation, then by uid for shoots created in the
// same second.
func olderShoot(a, b gardener.ShootInfo) bool {
	if !a.Created.Equal(b.Created) {
		return a.Created.Before(b.Created)
	}
	return a.UID < b.UID
}

// minorVersion cuts a Kubernetes version to major.minor: "1.31.4" -> "1.31".
func minorVersion(v string) string {
	major, rest, ok := strings.Cut(v, ".")
	if !ok {
		return v
	}
	minor, _, _ := strings.Cut(rest, ".")
	return major + "." + minor
}

// orphans are the shoots in Gardener without an active cluster.
type orphans struct {
	// unknown shoots have no cluster row at all.
	unknown []uuid.UUID
	// deleted shoots belong to a soft-deleted cluster.
	deleted []uuid.UUID
	// toConfirm and toStart are the shoots of deleted clusters to delete:
	// those whose deletion has started and those whose deletion has not.
	toConfirm, toStart []uuid.UUID
}

// findOrphans classifies the shoots without an active cluster. Only a shoot
// whose cluster row exists and is soft-deleted is to be deleted: a user asked
// for that and the delete sync did not finish it. A shoot without a cluster
// row is reported and left alone, since an empty, restored or wrong database
// or another installation sharing this Gardener looks exactly the same.
func (h *Handler) findOrphans(ctx context.Context, dbClusterByID map[uuid.UUID]db.ClusterListActiveRow, shootByClusterID map[uuid.UUID]gardener.ShootInfo) (orphans, error) {
	var orphanIDs []uuid.UUID
	for clusterID := range shootByClusterID {
		if _, exists := dbClusterByID[clusterID]; !exists {
			orphanIDs = append(orphanIDs, clusterID)
		}
	}
	if len(orphanIDs) == 0 {
		return orphans{}, nil
	}
	slices.SortFunc(orphanIDs, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })

	deletedClusters, err := h.queries.ClusterListDeletedByIDs(ctx, db.ClusterListDeletedByIDsParams{ClusterIds: orphanIDs})
	if err != nil {
		h.logger.Error("failed to list deleted clusters from DB", "error", err)
		return orphans{}, fmt.Errorf("list deleted clusters: %w", err)
	}

	var o orphans
	deleted := make(map[uuid.UUID]bool, len(deletedClusters))
	// Shoots already being deleted are only confirmed again: their deletion was
	// started before, and repeating it only adds the confirmations Gardener may
	// still need. They do not count towards the limit.
	for _, c := range deletedClusters {
		deleted[c.ID] = true
		o.deleted = append(o.deleted, c.ID)
		// The delete sync is still working on this cluster; it deletes the
		// shoot itself, or settles as failed and leaves it to a later pass.
		if c.SyncRunning {
			continue
		}
		if shootByClusterID[c.ID].Deleting {
			o.toConfirm = append(o.toConfirm, c.ID)
		} else {
			o.toStart = append(o.toStart, c.ID)
		}
	}

	for _, clusterID := range orphanIDs {
		if !deleted[clusterID] {
			shoot := shootByClusterID[clusterID]
			h.logger.Warn("shoot has no cluster in the database, not deleting it",
				"shoot", shoot.Name, "namespace", shoot.Namespace, "cluster_id", clusterID)
			o.unknown = append(o.unknown, clusterID)
		}
	}

	return o, nil
}

// deleteOrphans deletes the shoots of deleted clusters. Refusing does not fail
// the pass: the reconcile worker exits after a few failed passes, which would
// stop the outbox and status workers as well.
func (h *Handler) deleteOrphans(ctx context.Context, o orphans, shootByClusterID map[uuid.UUID]gardener.ShootInfo) {
	toStart := o.toStart
	if len(toStart) > h.cfg.MaxReconcileDeletions {
		h.logger.Error("refusing to delete shoots: too many in one pass, check the database",
			"shoots_to_delete", len(toStart),
			"max", h.cfg.MaxReconcileDeletions,
			"cluster_ids", toStart)
		toStart = nil
	}

	for _, clusterID := range slices.Concat(o.toConfirm, toStart) {
		shoot := shootByClusterID[clusterID]
		h.logger.Warn("deleting shoot of deleted cluster in Gardener",
			"shoot", shoot.Name, "cluster_id", clusterID)
		if _, err := h.gardener.DeleteShootByClusterID(ctx, clusterID); err != nil {
			h.logger.Error("failed to delete shoot of deleted cluster",
				"shoot", shoot.Name, "error", err)
		}
	}
}

// recordObservation stores, in one transaction, what one successful
// ListShoots showed: the shoots it returned (those it no longer returned are
// soft-deleted), the drift findings of this pass (those it no longer found
// are resolved) and that the pass completed. It is only called after the
// list succeeded: a failed list says nothing about which shoots are gone.
func (h *Handler) recordObservation(ctx context.Context, shoots []gardener.ShootInfo, findings []driftFinding) error {
	seen := pgtype.Timestamptz{Time: time.Now(), Valid: true}

	// Non-nil slices: pgx sends a nil slice as NULL, and ANY(NULL) would mark
	// nothing gone when Gardener lists no shoots at all.
	n := len(shoots)
	params := db.GardenerShootUpsertParams{
		Seen:               seen,
		ShootUids:          make([]string, 0, n),
		ShootNames:         make([]string, 0, n),
		ShootNamespaces:    make([]string, 0, n),
		ClusterIds:         make([]uuid.UUID, 0, n),
		KubernetesVersions: make([]string, 0, n),
		Regions:            make([]string, 0, n),
		ShootCreated:       make([]pgtype.Timestamptz, 0, n),
	}
	for _, s := range shoots {
		// A shoot without a cluster id is reported as a finding only.
		if s.InvalidClusterID {
			continue
		}
		params.ShootUids = append(params.ShootUids, s.UID)
		params.ShootNames = append(params.ShootNames, s.Name)
		params.ShootNamespaces = append(params.ShootNamespaces, s.Namespace)
		params.ClusterIds = append(params.ClusterIds, s.ClusterID)
		params.KubernetesVersions = append(params.KubernetesVersions, s.KubernetesVersion)
		params.Regions = append(params.Regions, s.Region)
		params.ShootCreated = append(params.ShootCreated, pgtype.Timestamptz{Time: s.Created, Valid: true})
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := h.queries.WithTx(tx)

	if len(params.ShootUids) > 0 {
		if err := q.GardenerShootUpsert(ctx, params); err != nil {
			return fmt.Errorf("upsert shoots: %w", err)
		}
	}
	// An empty list is a real answer too: every shoot recorded so far is gone.
	gone, err := q.GardenerShootMarkGone(ctx, db.GardenerShootMarkGoneParams{
		Seen:      seen,
		ShootUids: params.ShootUids,
	})
	if err != nil {
		return fmt.Errorf("mark gone shoots: %w", err)
	}

	if len(findings) > 0 {
		if err := q.DriftFindingUpsert(ctx, driftFindingParams(findings, seen)); err != nil {
			return fmt.Errorf("upsert drift findings: %w", err)
		}
	}
	resolved, err := q.DriftFindingResolve(ctx, db.DriftFindingResolveParams{Seen: seen})
	if err != nil {
		return fmt.Errorf("resolve drift findings: %w", err)
	}

	if err := q.WorkerHeartbeatRecord(ctx, db.WorkerHeartbeatRecordParams{
		Worker:          string(dbconst.WorkerHeartbeatWorker_Reconcile),
		IntervalSeconds: int32(h.cfg.ReconcileInterval.Seconds()),
		LastPass:        seen,
	}); err != nil {
		return fmt.Errorf("record pass: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	if gone > 0 {
		h.logger.Info("shoots gone from Gardener", "count", gone)
	}
	if resolved > 0 {
		h.logger.Info("drift findings resolved", "count", resolved)
	}
	return nil
}

// driftFindingParams zips the findings into the upsert's arrays. The empty
// string stands for NULL.
func driftFindingParams(findings []driftFinding, seen pgtype.Timestamptz) db.DriftFindingUpsertParams {
	n := len(findings)
	params := db.DriftFindingUpsertParams{
		Seen:            seen,
		Kinds:           make([]string, 0, n),
		ClusterIds:      make([]string, 0, n),
		ShootUids:       make([]string, 0, n),
		ShootNames:      make([]string, 0, n),
		ShootNamespaces: make([]string, 0, n),
		Expected:        make([]string, 0, n),
		Observed:        make([]string, 0, n),
	}
	for _, f := range findings {
		clusterID := ""
		if f.clusterID != uuid.Nil {
			clusterID = f.clusterID.String()
		}
		params.Kinds = append(params.Kinds, string(f.kind))
		params.ClusterIds = append(params.ClusterIds, clusterID)
		params.ShootUids = append(params.ShootUids, f.shoot.UID)
		params.ShootNames = append(params.ShootNames, f.shoot.Name)
		params.ShootNamespaces = append(params.ShootNamespaces, f.shoot.Namespace)
		params.Expected = append(params.Expected, f.expected)
		params.Observed = append(params.Observed, f.observed)
	}
	return params
}
