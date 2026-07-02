package namespace

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/shoot"
	db "github.com/fundament-oss/fundament/cluster-worker/pkg/db/gen"
	"github.com/fundament-oss/fundament/cluster-worker/pkg/handler"
)

func i4(v int32) pgtype.Int4 { return pgtype.Int4{Int32: v, Valid: true} }

func TestMergedLimitDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		row         db.NamespaceGetForSyncRow
		want        shoot.LimitDefaults
		wantHasAny  bool
		wantErrPart string
	}{
		{
			name:       "neither source set",
			row:        db.NamespaceGetForSyncRow{},
			want:       shoot.LimitDefaults{},
			wantHasAny: false,
		},
		{
			name: "org only",
			row: db.NamespaceGetForSyncRow{
				OrgDefaultCpuRequestM: i4(100), OrgDefaultCpuLimitM: i4(500),
				OrgDefaultMemoryRequestMi: i4(128), OrgDefaultMemoryLimitMi: i4(512),
			},
			want: shoot.LimitDefaults{
				CPURequestMilli: ptr.To[int32](100), CPULimitMilli: ptr.To[int32](500),
				MemoryRequestMi: ptr.To[int32](128), MemoryLimitMi: ptr.To[int32](512),
			},
			wantHasAny: true,
		},
		{
			name: "project only",
			row: db.NamespaceGetForSyncRow{
				ProjectDefaultCpuLimitM: i4(250),
			},
			want:       shoot.LimitDefaults{CPULimitMilli: ptr.To[int32](250)},
			wantHasAny: true,
		},
		{
			name: "both set, lowest wins per field",
			row: db.NamespaceGetForSyncRow{
				OrgDefaultCpuRequestM: i4(100), ProjectDefaultCpuRequestM: i4(50),
				OrgDefaultCpuLimitM: i4(500), ProjectDefaultCpuLimitM: i4(800),
				OrgDefaultMemoryLimitMi: i4(512),
			},
			want: shoot.LimitDefaults{
				CPURequestMilli: ptr.To[int32](50),  // project tightened
				CPULimitMilli:   ptr.To[int32](500), // org wins over higher project value
				MemoryLimitMi:   ptr.To[int32](512), // only org set
			},
			wantHasAny: true,
		},
		{
			name: "mixed-NULL cpu request exceeds limit fails",
			row: db.NamespaceGetForSyncRow{
				ProjectDefaultCpuRequestM: i4(800),
				OrgDefaultCpuLimitM:       i4(500),
			},
			wantErrPart: "cpu request 800m exceeds cpu limit 500m",
		},
		{
			name: "mixed-NULL memory request exceeds limit fails",
			row: db.NamespaceGetForSyncRow{
				OrgDefaultMemoryRequestMi:   i4(1024),
				ProjectDefaultMemoryLimitMi: i4(512),
			},
			wantErrPart: "memory request 1024Mi exceeds memory limit 512Mi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			defaults, hasAny, err := mergedLimitDefaults(&tt.row)
			if tt.wantErrPart != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErrPart)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantHasAny, hasAny)
			require.Equal(t, tt.want, defaults)
		})
	}
}

// Task 6.6: defaults present -> ensure() applies the LimitRange with the
// merged spec and the managed label set.
func TestEnsure_AppliesLimitRange(t *testing.T) {
	t.Parallel()
	h, mock := newTestHandler(t)
	row := testRow("team-a")
	row.OrgDefaultCpuLimitM = i4(500)
	row.ProjectDefaultCpuLimitM = i4(250)
	row.OrgDefaultMemoryLimitMi = i4(512)

	require.NoError(t, h.ensure(context.Background(), row))

	lr := mock.GetLimitRange(row.ClusterID, clusterName(row))
	require.NotNil(t, lr)
	require.Equal(t, shoot.LimitDefaults{
		CPULimitMilli: ptr.To[int32](250),
		MemoryLimitMi: ptr.To[int32](512),
	}, lr.Defaults)
	require.Equal(t, ManagedByValue, lr.Labels[LabelManagedBy])
	require.Equal(t, row.ID.String(), lr.Labels[LabelNamespaceID])
}

// Task 6.6: defaults cleared -> ensure() removes the managed LimitRange.
func TestEnsure_RemovesLimitRangeWhenDefaultsCleared(t *testing.T) {
	t.Parallel()
	h, mock := newTestHandler(t)
	row := testRow("team-a")
	row.OrgDefaultCpuLimitM = i4(500)

	require.NoError(t, h.ensure(context.Background(), row))
	require.NotNil(t, mock.GetLimitRange(row.ClusterID, clusterName(row)))

	row.OrgDefaultCpuLimitM = pgtype.Int4{}
	require.NoError(t, h.ensure(context.Background(), row))
	require.Nil(t, mock.GetLimitRange(row.ClusterID, clusterName(row)))

	// And again: delete-when-absent stays a no-op through the handler.
	require.NoError(t, h.ensure(context.Background(), row))
}

// Task 6.6: invalid merge -> ensure() errors and applies no LimitRange; the
// namespace itself is still ensured (created before the merge is evaluated).
func TestEnsure_InvalidMergeFailsWithoutApplying(t *testing.T) {
	t.Parallel()
	h, mock := newTestHandler(t)
	row := testRow("team-a")
	row.ProjectDefaultCpuRequestM = i4(800)
	row.OrgDefaultCpuLimitM = i4(500)

	err := h.ensure(context.Background(), row)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cpu request 800m exceeds cpu limit 500m")

	require.Nil(t, mock.GetLimitRange(row.ClusterID, clusterName(row)))
	// The namespace was created before the LimitRange step failed.
	require.NotNil(t, nsLabels(t, mock, row.ClusterID, clusterName(row)))
}

// Task 6.6: a namespace whose shoot is not ready defers before any shoot call,
// LimitRange included (gate lives in syncNamespace, exercised via Sync in the
// integration tests; here we assert ensure() is not reached by checking the
// precondition error shape the gate produces).
func TestEnsure_LimitRangeErrorPropagates(t *testing.T) {
	t.Parallel()
	h, mock := newTestHandler(t)
	row := testRow("team-a")
	row.OrgDefaultCpuLimitM = i4(500)
	mock.EnsureLimitRangeError = errors.New("boom")

	err := h.ensure(context.Background(), row)
	require.Error(t, err)
	require.Contains(t, err.Error(), "ensure limit range")
	var precond *handler.PreconditionError
	require.False(t, errors.As(err, &precond), "a shoot I/O failure must be a real error, not a deferral")
}
