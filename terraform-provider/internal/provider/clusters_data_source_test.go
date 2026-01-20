package provider

import (
	"testing"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

func TestClusterStatusToString(t *testing.T) {
	tests := []struct {
		name     string
		status   organizationv1.ClusterStatus
		expected string
	}{
		{
			name:     "unspecified status",
			status:   organizationv1.ClusterStatus_CLUSTER_STATUS_UNSPECIFIED,
			expected: "unspecified",
		},
		{
			name:     "provisioning status",
			status:   organizationv1.ClusterStatus_CLUSTER_STATUS_PROVISIONING,
			expected: "provisioning",
		},
		{
			name:     "starting status",
			status:   organizationv1.ClusterStatus_CLUSTER_STATUS_STARTING,
			expected: "starting",
		},
		{
			name:     "running status",
			status:   organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING,
			expected: "running",
		},
		{
			name:     "upgrading status",
			status:   organizationv1.ClusterStatus_CLUSTER_STATUS_UPGRADING,
			expected: "upgrading",
		},
		{
			name:     "error status",
			status:   organizationv1.ClusterStatus_CLUSTER_STATUS_ERROR,
			expected: "error",
		},
		{
			name:     "stopping status",
			status:   organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPING,
			expected: "stopping",
		},
		{
			name:     "stopped status",
			status:   organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPED,
			expected: "stopped",
		},
		{
			name:     "unknown status defaults to unspecified",
			status:   organizationv1.ClusterStatus(999),
			expected: "unspecified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := clusterStatusToString(tt.status)
			if result != tt.expected {
				t.Errorf("clusterStatusToString(%v) = %q, want %q", tt.status, result, tt.expected)
			}
		})
	}
}
