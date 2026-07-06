package namespace

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
)

// mergedLimitDefaults computes the namespace's effective per-container
// resource defaults from the raw org/project limits columns: per field the
// lowest non-NULL value wins, so a project default can only tighten the
// organization default. hasAny reports whether any field is set.
//
// The request/limit guard only triggers on mixed-NULL combinations (e.g. the
// org sets only a limit and the project only a higher request): when both
// sources define both bounds, min(request) <= min(limit) follows from each row
// satisfying request <= limit. The kube-apiserver would reject such a
// LimitRange, so the sync fails visibly instead of applying it.
func mergedLimitDefaults(row *db.NamespaceGetForSyncRow) (defaults shoot.LimitDefaults, hasAny bool, err error) {
	defaults = shoot.LimitDefaults{
		CPURequestMilli: leastInt4(row.ProjectDefaultCpuRequestM, row.OrgDefaultCpuRequestM),
		CPULimitMilli:   leastInt4(row.ProjectDefaultCpuLimitM, row.OrgDefaultCpuLimitM),
		MemoryRequestMi: leastInt4(row.ProjectDefaultMemoryRequestMi, row.OrgDefaultMemoryRequestMi),
		MemoryLimitMi:   leastInt4(row.ProjectDefaultMemoryLimitMi, row.OrgDefaultMemoryLimitMi),
	}

	if defaults.CPURequestMilli != nil && defaults.CPULimitMilli != nil && *defaults.CPURequestMilli > *defaults.CPULimitMilli {
		return shoot.LimitDefaults{}, false, fmt.Errorf(
			"invalid merged resource defaults: cpu request %dm exceeds cpu limit %dm",
			*defaults.CPURequestMilli, *defaults.CPULimitMilli)
	}
	if defaults.MemoryRequestMi != nil && defaults.MemoryLimitMi != nil && *defaults.MemoryRequestMi > *defaults.MemoryLimitMi {
		return shoot.LimitDefaults{}, false, fmt.Errorf(
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

// reconcileLimitRange materializes the merged resource defaults as the managed
// fundament-defaults LimitRange in the (already ensured) namespace, or removes
// it when no defaults apply. Runs inside the namespace ensure path, so it
// inherits its shoot-readiness gate and namespace-before-LimitRange ordering.
func (h *Handler) reconcileLimitRange(ctx context.Context, row *db.NamespaceGetForSyncRow, name string) error {
	defaults, hasAny, err := mergedLimitDefaults(row)
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
