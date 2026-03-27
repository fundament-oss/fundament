package cluster

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
)

// resolveProjectNamespace ensures the Gardener project exists and returns its namespace.
// Returns the namespace string (empty if not ready yet) or an error.
func (h *Handler) resolveProjectNamespace(ctx context.Context, orgName string, orgID uuid.UUID) (string, error) {
	projectName := gardener.ProjectName(orgName)
	namespace, err := h.gardener.EnsureProject(ctx, projectName, orgID)
	if err != nil {
		return "", fmt.Errorf("ensure project %s: %w", projectName, err)
	}
	return namespace, nil
}

// clusterToSyncBase builds a ClusterToSync with the common fields shared across
// sync, status checking, and deleted cluster verification.
// Callers set additional fields (ShootName, Deleted, NodePools) as needed.
func clusterToSyncBase(id uuid.UUID, name, orgName string, orgID uuid.UUID, namespace, region, k8sVersion string) *gardener.ClusterToSync {
	return &gardener.ClusterToSync{
		ID:                id,
		OrganizationID:    orgID,
		OrganizationName:  orgName,
		Name:              name,
		Namespace:         namespace,
		Region:            region,
		KubernetesVersion: k8sVersion,
	}
}
