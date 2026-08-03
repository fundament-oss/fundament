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
	v := RookValues()
	assert.Equal(t, "true", v["enableDiscoveryDaemon"])
}

func TestBootstrapCephClusterIsEmpty(t *testing.T) {
	u := BootstrapCephCluster("rook-ceph")
	assert.Equal(t, "CephCluster", u.GetKind())
	useAll, _, _ := unstructuredNestedBool(u.Object, "spec", "storage", "useAllDevices")
	assert.False(t, useAll)
	nodes, found, _ := unstructuredNestedSlice(u.Object, "spec", "storage", "nodes")
	assert.True(t, !found || len(nodes) == 0)
}
