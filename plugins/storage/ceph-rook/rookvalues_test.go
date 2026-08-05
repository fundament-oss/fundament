package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func unstructuredNestedBool(obj map[string]any, fields ...string) (bool, bool, error) {
	val, found, err := nestedField(obj, fields...)
	if !found || err != nil {
		return false, found, err
	}
	b, ok := val.(bool)
	if !ok {
		return false, false, nil
	}
	return b, true, nil
}

func unstructuredNestedSlice(obj map[string]any, fields ...string) ([]any, bool, error) {
	val, found, err := nestedField(obj, fields...)
	if !found || err != nil {
		return nil, found, err
	}
	s, ok := val.([]any)
	if !ok {
		return nil, false, nil
	}
	return s, true, nil
}

func nestedField(obj map[string]any, fields ...string) (any, bool, error) {
	cur := obj
	for i, f := range fields {
		v, ok := cur[f]
		if !ok {
			return nil, false, nil
		}
		if i == len(fields)-1 {
			return v, true, nil
		}
		cur, ok = v.(map[string]any)
		if !ok {
			return nil, false, nil
		}
	}
	return nil, false, nil
}

func testConfig() Config {
	return Config{
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

func TestRookValuesLoopDevices(t *testing.T) {
	cfg := testConfig()
	cfg.DevLoopDevices = true
	v := RookValues(cfg)
	assert.Equal(t, "true", v["allowLoopDevices"])
	// discoverDaemonUdev only filters udev events, never ConfigMap contents,
	// and setting it would drop Rook's own dm/rbd/nbd defaults.
	assert.NotContains(t, v, "discoverDaemonUdev")
}

func TestBootstrapCephClusterIsEmpty(t *testing.T) {
	u := BootstrapCephCluster("rook-ceph", testConfig())
	assert.Equal(t, "CephCluster", u.GetKind())
	// A privileged k3d node exposes the host's disks, so this must never be true.
	useAll, _, _ := unstructuredNestedBool(u.Object, "spec", "storage", "useAllDevices")
	assert.False(t, useAll)
	nodes, found, _ := unstructuredNestedSlice(u.Object, "spec", "storage", "nodes")
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
	multi, _, _ := unstructuredNestedBool(u.Object, "spec", "mon", "allowMultiplePerNode")
	assert.True(t, multi)
	multi, _, _ = unstructuredNestedBool(u.Object, "spec", "mgr", "allowMultiplePerNode")
	assert.True(t, multi)

	cfg.CephImage = "quay.io/ceph/ceph:v19.2.3"
	image, _, _ := unstructured.NestedString(BootstrapCephCluster("rook-ceph", cfg).Object,
		"spec", "cephVersion", "image")
	assert.Equal(t, "quay.io/ceph/ceph:v19.2.3", image)
}
