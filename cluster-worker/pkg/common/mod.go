package common

import (
	"log/slog"
	"os"

	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/gardener"
)

// TestCluster creates a logger for tests.
func TestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}

// TestCluster creates a valid ClusterToSync for testing.
// Uses the new naming scheme with deterministic project names and random shoot names.
// Includes a default node pool.
func TestCluster(name, org string) gardener.ClusterToSync {
	orgID := uuid.New()
	projectName := gardener.ProjectName(org)
	namespace := gardener.NamespaceFromProjectName(projectName)
	shootName := gardener.GenerateShootName(name)
	return gardener.ClusterToSync{
		ID:                uuid.New(),
		OrganizationID:    orgID,
		OrganizationName:  org,
		Name:              name,
		ShootName:         shootName,
		Namespace:         namespace,
		Region:            "local",
		KubernetesVersion: "1.31.1",
		NodePools: []gardener.NodePool{
			{
				Name:         "default",
				MachineType:  "local",
				AutoscaleMin: 1,
				AutoscaleMax: 3,
			},
		},
	}
}

// TestClusterWithoutNodePools creates a test cluster with zero node pools
// to test the fallback behavior.
func TestClusterWithoutNodePools(name, org string) gardener.ClusterToSync {
	cluster := TestCluster(name, org)
	cluster.NodePools = nil
	return cluster
}
