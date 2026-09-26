package main

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/observability"
)

// fakeHost records the last reported status; the reconciler only calls
// ReportStatus, so Telemetry can stay nil.
type fakeHost struct {
	last  pluginruntime.PluginStatus
	calls int
}

func (h *fakeHost) Logger() *slog.Logger                      { return slog.Default() }
func (h *fakeHost) Telemetry() observability.TelemetryService { return nil }
func (h *fakeHost) ReportReady()                              {}
func (h *fakeHost) ReportStatus(s pluginruntime.PluginStatus) { h.last = s; h.calls++ }

func ccStatus(t *testing.T, phase, health string, details map[string]any) *unstructured.Unstructured {
	t.Helper()
	u := rookStub("CephCluster")
	u.SetName(cephClusterName)
	u.SetNamespace(testNamespace)
	if phase != "" {
		require.NoError(t, unstructured.SetNestedField(u.Object, phase, "status", "phase"))
	}
	if health != "" {
		require.NoError(t, unstructured.SetNestedField(u.Object, health, "status", "ceph", "health"))
	}
	if details != nil {
		require.NoError(t, unstructured.SetNestedMap(u.Object, details, "status", "ceph", "details"))
	}
	return u
}

func TestCephClusterPluginStatus(t *testing.T) {
	t.Parallel()

	// Ready + HEALTH_OK is the plain healthy case.
	s := cephClusterPluginStatus(ccStatus(t, "Ready", "HEALTH_OK", nil))
	assert.Equal(t, pluginruntime.PhaseRunning, s.Phase)

	// HEALTH_WARN is not degraded: a single-node cluster warns about redundancy
	// forever yet serves storage.
	s = cephClusterPluginStatus(ccStatus(t, "Ready", "HEALTH_WARN", nil))
	assert.Equal(t, pluginruntime.PhaseRunning, s.Phase)

	// HEALTH_ERR degrades and names a cause from details.
	s = cephClusterPluginStatus(ccStatus(t, "Ready", "HEALTH_ERR", map[string]any{
		"OSD_DOWN": map[string]any{"message": "1 osds down"},
	}))
	assert.Equal(t, pluginruntime.PhaseDegraded, s.Phase)
	assert.Contains(t, s.Message, "HEALTH_ERR")
	assert.Contains(t, s.Message, "1 osds down")

	// Not Ready degrades and carries the cluster's own message.
	s = cephClusterPluginStatus(ccStatus(t, "Progressing", "", nil))
	assert.Equal(t, pluginruntime.PhaseDegraded, s.Phase)
	assert.Contains(t, s.Message, "Progressing")

	// A cluster with no status yet still reads as not-ready, legibly.
	s = cephClusterPluginStatus(ccStatus(t, "", "", nil))
	assert.Equal(t, pluginruntime.PhaseDegraded, s.Phase)
	assert.Contains(t, s.Message, "no status yet")
}

func TestCephHealthReconcilerReportsHealth(t *testing.T) {
	t.Parallel()
	cc := ccStatus(t, "Ready", "HEALTH_OK", nil)
	c := newFakeClient(t, cc)
	host := &fakeHost{}
	r := &CephHealthReconciler{Client: c, ClusterNamespace: testNamespace, Host: host}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: cephClusterName},
	})
	require.NoError(t, err)
	assert.Equal(t, pluginruntime.PhaseRunning, host.last.Phase)
}

func TestCephHealthReconcilerDegradesWhenClusterMissing(t *testing.T) {
	t.Parallel()
	c := newFakeClient(t) // no CephCluster
	host := &fakeHost{}
	r := &CephHealthReconciler{Client: c, ClusterNamespace: testNamespace, Host: host}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: cephClusterName},
	})
	require.NoError(t, err)
	assert.Equal(t, pluginruntime.PhaseDegraded, host.last.Phase)
	assert.Contains(t, host.last.Message, "not found")
}
