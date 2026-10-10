package namespace

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
)

// effectiveDefaults computes the namespace's per-container resource defaults
// from the cluster and project default_* columns. hasAny reports whether any
// field is set.
//
// Per field the lowest non-NULL value wins. The database rejects a project
// value above its cluster's, so for anything written through the API this is
// the same as "the project's value, else the cluster's"; taking the lower one
// also keeps rows written before those triggers existed within the cluster's
// defaults.
//
// The request/limit guard is the backstop for the rule the project trigger
// enforces at write time. It only triggers on mixed-NULL combinations (the
// cluster sets only a limit and the project only a higher request): when both
// sources define both bounds, min(request) <= min(limit) follows from each row
// satisfying its own request <= limit. The kube-apiserver would reject such a
// LimitRange, so the sync fails visibly instead of applying it.
func effectiveDefaults(row *db.NamespaceGetForSyncRow) (defaults shoot.ContainerDefaults, hasAny bool, err error) {
	defaults = shoot.ContainerDefaults{
		CPURequestMilli: leastInt4(row.ProjectDefaultCpuRequestM, row.ClusterDefaultCpuRequestM),
		CPULimitMilli:   leastInt4(row.ProjectDefaultCpuLimitM, row.ClusterDefaultCpuLimitM),
		MemoryRequestMi: leastInt4(row.ProjectDefaultMemoryRequestMi, row.ClusterDefaultMemoryRequestMi),
		MemoryLimitMi:   leastInt4(row.ProjectDefaultMemoryLimitMi, row.ClusterDefaultMemoryLimitMi),
	}

	if defaults.CPURequestMilli != nil && defaults.CPULimitMilli != nil && *defaults.CPURequestMilli > *defaults.CPULimitMilli {
		return shoot.ContainerDefaults{}, false, fmt.Errorf(
			"invalid merged resource defaults: cpu request %dm exceeds cpu limit %dm",
			*defaults.CPURequestMilli, *defaults.CPULimitMilli)
	}
	if defaults.MemoryRequestMi != nil && defaults.MemoryLimitMi != nil && *defaults.MemoryRequestMi > *defaults.MemoryLimitMi {
		return shoot.ContainerDefaults{}, false, fmt.Errorf(
			"invalid merged resource defaults: memory request %dMi exceeds memory limit %dMi",
			*defaults.MemoryRequestMi, *defaults.MemoryLimitMi)
	}

	hasAny = defaults.CPURequestMilli != nil || defaults.CPULimitMilli != nil ||
		defaults.MemoryRequestMi != nil || defaults.MemoryLimitMi != nil
	return defaults, hasAny, nil
}

// leastInt4 returns the smallest of the non-NULL values, or nil when both are
// NULL (mirroring SQL LEAST semantics).
func leastInt4(a, b pgtype.Int4) *int32 {
	switch {
	case a.Valid && b.Valid:
		if a.Int32 <= b.Int32 {
			return &a.Int32
		}
		return &b.Int32
	case a.Valid:
		return &a.Int32
	case b.Valid:
		return &b.Int32
	default:
		return nil
	}
}

// reconcileLimitRange materializes the effective resource defaults as the
// managed fundament-defaults LimitRange in the (already ensured) namespace, or
// removes it when no defaults apply. Runs inside the namespace ensure path, so
// it inherits its shoot-readiness gate and namespace-before-LimitRange
// ordering.
func (h *Handler) reconcileLimitRange(ctx context.Context, row *db.NamespaceGetForSyncRow, name string) error {
	defaults, hasAny, err := effectiveDefaults(row)
	if err != nil {
		return fmt.Errorf("namespace %s: %w", name, err)
	}

	if !hasAny {
		if err := h.shoot.DeleteLimitRange(ctx, row.ClusterID, name); err != nil {
			return fmt.Errorf("delete limit range in namespace %s: %w", name, err)
		}
		return nil
	}

	if err := h.shoot.EnsureLimitRange(ctx, row.ClusterID, name, defaults, desiredLabels(row)); err != nil {
		return fmt.Errorf("ensure limit range in namespace %s: %w", name, err)
	}
	return nil
}
