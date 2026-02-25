package common

import (
	"log/slog"
	"os"

	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
)

// TestCluster creates a logger for tests.
func TestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}

// TestCluster creates a valid ClusterToSync for testing.
// Uses the new naming scheme with deterministic project names and random shoot names.
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
	}
}
