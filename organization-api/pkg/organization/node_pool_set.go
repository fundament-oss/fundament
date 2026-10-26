package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/common/authz"
	"github.com/fundament-oss/fundament/common/dbconst"
	"github.com/fundament-oss/fundament/common/rollback"
	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// SetNodePools makes the request's pools the cluster's complete set, in one
// transaction. Pools are matched by name: an unchanged pool is left alone, a
// resized one is updated, one whose machine type changes is replaced, pools
// missing from the set are removed and new ones are created. cluster-worker
// only ever sees the committed result, so the cluster never passes through
// zero pools or an intermediate set. Removals and shrinks run before
// additions and growth, so every quota check along the way stays at or below
// the result's total.
func (s *Server) SetNodePools(
	ctx context.Context,
	req *organizationv1.SetNodePoolsRequest,
) (*organizationv1.SetNodePoolsResponse, error) {
	clusterID := uuid.MustParse(req.GetClusterId())

	// Retry: the add-cluster wizard sets the pools right after creating the
	// cluster, before its authz tuple has synced (see checkPermissionWithRetry).
	// A cluster admin may create, edit and delete its pools alike.
	if err := s.checkPermissionWithRetry(ctx, authz.CanCreateNodePool(), authz.Cluster(clusterID)); err != nil {
		return nil, err
	}

	// A live cluster keeps a pool that can run nodes (see node_pool_guard.go).
	active := false
	for _, spec := range req.GetNodePools() {
		if spec.GetAutoscaleMax() > 0 {
			active = true
		}
	}
	if !active {
		return nil, failedPrecondition(msgNoActivePool)
	}

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to begin transaction: %w", err))
	}
	defer rollback.Rollback(ctx, tx, s.logger)

	qtx := s.queries.WithTx(tx)

	cluster, err := qtx.ClusterLockForNodePools(ctx, db.ClusterLockForNodePoolsParams{ClusterID: clusterID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("cluster not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to lock cluster: %w", err))
	}
	if cluster.Deleted.Valid {
		return nil, failedPrecondition(msgClusterDeleting)
	}

	current, err := qtx.NodePoolListByClusterID(ctx, db.NodePoolListByClusterIDParams{ClusterID: clusterID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list node pools: %w", err))
	}
	byName := make(map[string]db.NodePoolListByClusterIDRow, len(current))
	for _, pool := range current {
		byName[pool.Name] = pool
	}

	// Aborted, the code for a concurrency conflict: the caller read an older
	// set, and can tell this refusal from the others by its code.
	if req.HasExpectedCurrent() && !samePoolIDs(current, req.GetExpectedCurrent().GetNodePoolIds()) {
		return nil, connect.NewError(connect.CodeAborted,
			fmt.Errorf("the cluster's node pools changed since they were read; reload and try again"))
	}

	type planned struct {
		spec     *organizationv1.NodePoolSpec
		offering uuid.UUID
	}
	var (
		creates, grows, shrinks []planned
		removes                 []uuid.UUID
		wanted                  = make(map[string]bool, len(req.GetNodePools()))
		toResolve               []string
	)
	for _, spec := range req.GetNodePools() {
		wanted[spec.GetName()] = true

		// A pool that keeps its name and machine type is resized at most, and
		// needs no catalog lookup: it may be one from before the catalog, or of
		// a type the region no longer offers, and keeps running as it is.
		existing, ok := byName[spec.GetName()]
		if ok && existing.MachineType == spec.GetMachineType() {
			switch {
			case existing.AutoscaleMin == spec.GetAutoscaleMin() && existing.AutoscaleMax == spec.GetAutoscaleMax():
				// unchanged
			case spec.GetAutoscaleMax() <= existing.AutoscaleMax:
				shrinks = append(shrinks, planned{spec: spec})
			default:
				grows = append(grows, planned{spec: spec})
			}
			continue
		}

		// A new pool, or a replacement under the same name.
		if ok {
			removes = append(removes, existing.ID)
		}
		creates = append(creates, planned{spec: spec})
		toResolve = append(toResolve, spec.GetMachineType())
	}

	// New pools resolve against the catalog within the cluster's region, where
	// only offered types are valid (pre-validate with ListRegions): one lookup
	// for all of them.
	offerings := make(map[string]uuid.UUID, len(toResolve))
	if len(toResolve) > 0 {
		rows, err := qtx.RegionMachineTypesResolve(ctx, db.RegionMachineTypesResolveParams{
			RegionName:       cluster.Region,
			MachineTypeNames: toResolve,
		})
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to resolve machine types: %w", err))
		}
		for _, row := range rows {
			offerings[row.Name] = row.ID
		}
	}
	for i := range creates {
		offering, ok := offerings[creates[i].spec.GetMachineType()]
		if !ok {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("machine type %q is not offered in region %q", creates[i].spec.GetMachineType(), cluster.Region))
		}
		creates[i].offering = offering
	}
	for _, pool := range current {
		if !wanted[pool.Name] {
			removes = append(removes, pool.ID)
		}
	}

	for _, id := range removes {
		if _, err := qtx.NodePoolDelete(ctx, db.NodePoolDeleteParams{ID: id}); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete node pool: %w", err))
		}
	}
	update := func(p planned) error {
		_, err := qtx.NodePoolUpdate(ctx, db.NodePoolUpdateParams{
			ID:           byName[p.spec.GetName()].ID,
			AutoscaleMin: pgtype.Int4{Int32: p.spec.GetAutoscaleMin(), Valid: true},
			AutoscaleMax: pgtype.Int4{Int32: p.spec.GetAutoscaleMax(), Valid: true},
		})
		if err != nil {
			return nodePoolWriteError(err, p.spec.GetMachineType(), cluster.Region, "update")
		}
		return nil
	}
	for _, p := range shrinks {
		if err := update(p); err != nil {
			return nil, err
		}
	}
	for _, p := range creates {
		_, err := qtx.NodePoolCreate(ctx, db.NodePoolCreateParams{
			ClusterID:           clusterID,
			Name:                p.spec.GetName(),
			MachineType:         p.spec.GetMachineType(),
			RegionMachineTypeID: pgtype.UUID{Bytes: p.offering, Valid: true},
			AutoscaleMin:        p.spec.GetAutoscaleMin(),
			AutoscaleMax:        p.spec.GetAutoscaleMax(),
		})
		if err != nil {
			return nil, nodePoolWriteError(err, p.spec.GetMachineType(), cluster.Region, "create")
		}
	}
	for _, p := range grows {
		if err := update(p); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to commit: %w", err))
	}

	s.logger.InfoContext(ctx, "node pools set",
		"cluster_id", clusterID,
		"created", len(creates),
		"removed", len(removes),
		"resized", len(shrinks)+len(grows),
	)

	return organizationv1.SetNodePoolsResponse_builder{}.Build(), nil
}

// samePoolIDs reports whether the cluster's live pools are exactly the given ids.
func samePoolIDs(current []db.NodePoolListByClusterIDRow, expected []string) bool {
	if len(current) != len(expected) {
		return false
	}
	want := make(map[string]bool, len(expected))
	for _, id := range expected {
		want[id] = true
	}
	for _, pool := range current {
		if !want[pool.ID.String()] {
			return false
		}
	}
	return true
}

// nodePoolWriteError maps a node pool insert or update error to the API's
// error, for every handler that writes a pool. The trigger messages name the
// numbers and are written to be shown. machineType and region name the
// offering of an insert; an update has none to name.
func nodePoolWriteError(err error, machineType, region, action string) error {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch {
		// The quota trigger: the pool's maximum would take the organization
		// over its node quota for this machine type in this region.
		case pgErr.Code == pgerrcode.RaiseException && pgErr.Hint == dbconst.HintNodePoolQuotaExceeded:
			return connect.NewError(connect.CodeResourceExhausted, errors.New(pgErr.Message))
		// A pool from before the catalog whose machine type it does not offer.
		case pgErr.Code == pgerrcode.RaiseException && pgErr.Hint == dbconst.HintNodePoolQuotaUnresolvable:
			return connect.NewError(connect.CodeFailedPrecondition, errors.New(pgErr.Message))
		// The region-match constraint trigger: the cluster is not linked to the
		// region catalog (a pre-catalog cluster whose region_id is NULL).
		case pgErr.Code == pgerrcode.RaiseException && pgErr.Hint == dbconst.HintNodePoolRegionMismatch:
			return connect.NewError(connect.CodeFailedPrecondition,
				fmt.Errorf("the cluster is not linked to the region catalog; contact an operator"))
		case pgErr.Code == pgerrcode.ForeignKeyViolation && pgErr.ConstraintName == dbconst.ConstraintNodePoolsFkRegionMachineType:
			return connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("machine type %q is not offered in region %q", machineType, region))
		}
	}
	return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to %s node pool: %w", action, err))
}
