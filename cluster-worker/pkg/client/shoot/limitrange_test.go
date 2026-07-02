package shoot

import (
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

func TestLimitRangeSpec_AllFieldsFormatted(t *testing.T) {
	spec := limitRangeSpec(LimitDefaults{
		CPURequestMilli: ptr.To[int32](250),
		CPULimitMilli:   ptr.To[int32](500),
		MemoryRequestMi: ptr.To[int32](256),
		MemoryLimitMi:   ptr.To[int32](512),
	})

	require.Len(t, spec.Limits, 1)
	item := spec.Limits[0]
	require.Equal(t, corev1.LimitTypeContainer, item.Type)

	cpuLimit := item.Default[corev1.ResourceCPU]
	require.Equal(t, "500m", cpuLimit.String())
	memLimit := item.Default[corev1.ResourceMemory]
	require.Equal(t, "512Mi", memLimit.String())
	cpuRequest := item.DefaultRequest[corev1.ResourceCPU]
	require.Equal(t, "250m", cpuRequest.String())
	memRequest := item.DefaultRequest[corev1.ResourceMemory]
	require.Equal(t, "256Mi", memRequest.String())
}

func TestLimitRangeSpec_PartialFieldsOmitted(t *testing.T) {
	spec := limitRangeSpec(LimitDefaults{
		CPULimitMilli: ptr.To[int32](1500),
	})

	require.Len(t, spec.Limits, 1)
	item := spec.Limits[0]
	require.Nil(t, item.DefaultRequest, "no request values set")
	require.Len(t, item.Default, 1)
	cpuLimit := item.Default[corev1.ResourceCPU]
	require.Equal(t, "1500m", cpuLimit.String())
	require.NotContains(t, item.Default, corev1.ResourceMemory)
}

func TestMockLimitRangeRoundTrip(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	m := NewMockShootAccess(logger)
	clusterID := uuid.New()
	labels := map[string]string{"fundament.io/managed-by": "cluster-worker"}

	// Ensure creates.
	first := LimitDefaults{CPULimitMilli: ptr.To[int32](500)}
	require.NoError(t, m.EnsureLimitRange(t.Context(), clusterID, "team-a", first, labels))
	lr := m.GetLimitRange(clusterID, "team-a")
	require.NotNil(t, lr)
	require.Equal(t, first, lr.Defaults)
	require.Equal(t, labels, lr.Labels)

	// Ensure updates in place.
	second := LimitDefaults{CPULimitMilli: ptr.To[int32](250), MemoryLimitMi: ptr.To[int32](512)}
	require.NoError(t, m.EnsureLimitRange(t.Context(), clusterID, "team-a", second, labels))
	lr = m.GetLimitRange(clusterID, "team-a")
	require.NotNil(t, lr)
	require.Equal(t, second, lr.Defaults)

	// Delete removes.
	require.NoError(t, m.DeleteLimitRange(t.Context(), clusterID, "team-a"))
	require.Nil(t, m.GetLimitRange(clusterID, "team-a"))

	// Delete when absent is a no-op.
	require.NoError(t, m.DeleteLimitRange(t.Context(), clusterID, "team-a"))
}
