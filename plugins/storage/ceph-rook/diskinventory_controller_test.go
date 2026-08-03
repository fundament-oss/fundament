package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNodeFromConfigMap(t *testing.T) {
	tests := []struct {
		name   string
		cmName string
		labels map[string]string
		want   string
	}{
		{
			name:   "label takes precedence",
			cmName: "local-device-worker-1",
			labels: map[string]string{"rook.io/node": "worker-1"},
			want:   "worker-1",
		},
		{
			name:   "falls back to stripping local-device- prefix",
			cmName: "local-device-worker-2",
			labels: map[string]string{"app": "rook-discover"},
			want:   "worker-2",
		},
		{
			name:   "no prefix to strip returns name as-is",
			cmName: "some-other-cm",
			labels: map[string]string{},
			want:   "some-other-cm",
		},
		{
			name:   "empty node label falls back to name stripping",
			cmName: "local-device-node3",
			labels: map[string]string{"rook.io/node": ""},
			want:   "node3",
		},
		{
			name:   "nil labels falls back to name stripping",
			cmName: "local-device-node4",
			labels: nil,
			want:   "node4",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := nodeFromConfigMap(tc.cmName, tc.labels)
			assert.Equal(t, tc.want, got)
		})
	}
}
