package cluster

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
)

func TestShouldRefreshShootCA(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		current     gardener.ShootStatusType
		previous    gardener.ShootStatusType
		hasStoredCA bool
		wantRefresh bool
	}{
		{
			name:        "initial ready transition refreshes ca",
			current:     gardener.StatusReady,
			previous:    gardener.StatusProgressing,
			hasStoredCA: false,
			wantRefresh: true,
		},
		{
			name:        "ready cluster without stored ca retries",
			current:     gardener.StatusReady,
			previous:    gardener.StatusReady,
			hasStoredCA: false,
			wantRefresh: true,
		},
		{
			name:        "ready cluster with stored ca does not retry",
			current:     gardener.StatusReady,
			previous:    gardener.StatusReady,
			hasStoredCA: true,
			wantRefresh: false,
		},
		{
			name:        "non ready cluster does not refresh",
			current:     gardener.StatusProgressing,
			previous:    gardener.StatusReady,
			hasStoredCA: false,
			wantRefresh: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := shouldRefreshShootCA(tt.current, tt.previous, tt.hasStoredCA)
			require.Equal(t, tt.wantRefresh, got)
		})
	}
}
