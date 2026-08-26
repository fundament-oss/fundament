package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func unstructuredNestedBool(obj map[string]any, fields ...string) (bool, bool) {
	val, found := nestedField(obj, fields...)
	if !found {
		return false, false
	}
	b, ok := val.(bool)
	if !ok {
		return false, false
	}
	return b, true
}

func unstructuredNestedSlice(obj map[string]any, fields ...string) ([]any, bool) {
	val, found := nestedField(obj, fields...)
	if !found {
		return nil, false
	}
	s, ok := val.([]any)
	if !ok {
		return nil, false
	}
	return s, true
}

func nestedField(obj map[string]any, fields ...string) (any, bool) {
	cur := obj
	for i, f := range fields {
		v, ok := cur[f]
		if !ok {
			return nil, false
		}
		if i == len(fields)-1 {
			return v, true
		}
		cur, ok = v.(map[string]any)
		if !ok {
			return nil, false
		}
	}
	return nil, false
}

func testConfig() *Config {
	return &Config{
		CephImage: "quay.io/ceph/ceph:v18.2.4",
		MonCount:  3,
		MgrCount:  2,
	}
}

func TestRookValuesEnablesDiscovery(t *testing.T) {
	v := RookValues(testConfig())
	assert.Equal(t, "true", v["enableDiscoveryDaemon"])
	assert.NotContains(t, v, "allowLoopDevices")
	assert.NotContains(t, v, "discoverDaemonUdev")
}

// Left unset, Rook waits 60m before a newly racked disk shows up.
func TestRookValuesShortensDiscoveryInterval(t *testing.T) {
	v := RookValues(testConfig())
	assert.Equal(t, discoveryInterval, v["discoveryDaemonInterval"])
	assert.NotEqual(t, "60m", v["discoveryDaemonInterval"])
}

func TestRookValuesLoopDevices(t *testing.T) {
	cfg := testConfig()
	cfg.DevLoopDevices = true
	v := RookValues(cfg)
	assert.Equal(t, "true", v["allowLoopDevices"])
	// discoverDaemonUdev only filters udev events, and setting it would drop
	// Rook's own dm/rbd/nbd defaults.
	assert.NotContains(t, v, "discoverDaemonUdev")
}

func TestBootstrapCephClusterIsEmpty(t *testing.T) {
	u := BootstrapCephCluster("rook-ceph", testConfig())
	assert.Equal(t, "CephCluster", u.GetKind())
	// A privileged k3d node sees the host's disks, so neither may ever be true.
	useAllDevices, _ := unstructuredNestedBool(u.Object, "spec", "storage", "useAllDevices")
	assert.False(t, useAllDevices)
	useAllNodes, found := unstructuredNestedBool(u.Object, "spec", "storage", "useAllNodes")
	assert.True(t, found, "useAllNodes must be set explicitly, not left to Rook's default")
	assert.False(t, useAllNodes)
	nodes, found := unstructuredNestedSlice(u.Object, "spec", "storage", "nodes")
	assert.True(t, !found || len(nodes) == 0)

	dataDir, found, _ := unstructured.NestedString(u.Object, "spec", "dataDirHostPath")
	assert.True(t, found)
	assert.Equal(t, "/var/lib/rook", dataDir)

	image, _, _ := unstructured.NestedString(u.Object, "spec", "cephVersion", "image")
	assert.Equal(t, testConfig().CephImage, image)
}

func TestBootstrapCephClusterSingleNode(t *testing.T) {
	cfg := testConfig()
	cfg.MonCount, cfg.MgrCount, cfg.AllowMultiplePerNode = 1, 1, true
	u := BootstrapCephCluster("rook-ceph", cfg)

	monCount, _, _ := unstructured.NestedInt64(u.Object, "spec", "mon", "count")
	assert.Equal(t, int64(1), monCount)
	mgrCount, _, _ := unstructured.NestedInt64(u.Object, "spec", "mgr", "count")
	assert.Equal(t, int64(1), mgrCount)
	multi, _ := unstructuredNestedBool(u.Object, "spec", "mon", "allowMultiplePerNode")
	assert.True(t, multi)
	multi, _ = unstructuredNestedBool(u.Object, "spec", "mgr", "allowMultiplePerNode")
	assert.True(t, multi)

	cfg.CephImage = "quay.io/ceph/ceph:v19.2.3"
	image, _, _ := unstructured.NestedString(BootstrapCephCluster("rook-ceph", cfg).Object,
		"spec", "cephVersion", "image")
	assert.Equal(t, "quay.io/ceph/ceph:v19.2.3", image)
}

// Written unconditionally, not omitted when false, so an operator reading the
// live object can tell the guard is on rather than defaulted.
func TestBootstrapCephClusterAllowUnsupported(t *testing.T) {
	cfg := testConfig()
	cfg.AllowUnsupportedCeph = true
	allow, found := unstructuredNestedBool(BootstrapCephCluster("rook-ceph", cfg).Object,
		"spec", "cephVersion", "allowUnsupported")
	assert.True(t, found)
	assert.True(t, allow)

	cfg.AllowUnsupportedCeph = false
	allow, found = unstructuredNestedBool(BootstrapCephCluster("rook-ceph", cfg).Object,
		"spec", "cephVersion", "allowUnsupported")
	assert.True(t, found)
	assert.False(t, allow)
}

// The smoke script is what proves the environment works, so a plugin pinned
// elsewhere is validated by nothing.
func TestDefaultCephImageMatchesSmokeScript(t *testing.T) {
	cfg, err := LoadConfig()
	require.NoError(t, err)

	smoke, err := os.ReadFile("../../../deploy/k3d/rook-smoke.sh")
	require.NoError(t, err)
	assert.Contains(t, string(smoke), "CEPH_IMAGE:-"+cfg.CephImage,
		"rook-smoke.sh must default to the same Ceph image as the plugin")
}
