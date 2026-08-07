package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestRookValuesEnablesDiscovery(t *testing.T) {
	v := RookValues(false)
	assert.Equal(t, "true", v["enableDiscoveryDaemon"])
	_, hasLoop := v["allowLoopDevices"]
	assert.False(t, hasLoop, "allowLoopDevices must be absent by default")
}

func TestRookValuesAllowLoopDevices(t *testing.T) {
	v := RookValues(true)
	assert.Equal(t, "true", v["allowLoopDevices"])
}

func TestBootstrapCephClusterIsEmpty(t *testing.T) {
	u := BootstrapCephCluster("rook-ceph", 3)
	assert.Equal(t, "CephCluster", u.GetKind())
	useAll, _, _ := unstructuredNestedBool(u.Object, "spec", "storage", "useAllDevices")
	assert.False(t, useAll)
	nodes, found, _ := unstructuredNestedSlice(u.Object, "spec", "storage", "nodes")
	assert.True(t, !found || len(nodes) == 0)
	mon, _, _ := unstructuredNestedInt64(u.Object, "spec", "mon", "count")
	assert.Equal(t, int64(3), mon)
}

func TestBootstrapCephClusterSingleNode(t *testing.T) {
	u := BootstrapCephCluster("rook-ceph", 1)
	mon, _, _ := unstructuredNestedInt64(u.Object, "spec", "mon", "count")
	mgr, _, _ := unstructuredNestedInt64(u.Object, "spec", "mgr", "count")
	assert.Equal(t, int64(1), mon, "single node must use 1 mon so it schedules")
	assert.Equal(t, int64(1), mgr)
}

func TestMonMgrCountForNodes(t *testing.T) {
	tests := []struct {
		nodes, mon, mgr int
	}{
		{0, 1, 1}, {1, 1, 1}, {2, 1, 2}, {3, 3, 2}, {5, 3, 2},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.mon, monCountForNodes(tt.nodes), "mon for %d nodes", tt.nodes)
		assert.Equal(t, tt.mgr, mgrCountForNodes(tt.nodes), "mgr for %d nodes", tt.nodes)
	}
}
