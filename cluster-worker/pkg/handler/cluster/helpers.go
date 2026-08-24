package cluster

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fundament-oss/fundament/cluster-worker/pkg/client/gardener"
	"github.com/fundament-oss/fundament/common/kubename"
)

// resolveProjectNamespace ensures the Gardener project exists and returns its namespace.
// Returns the namespace string (empty if not ready yet) or an error.
func (h *Handler) resolveProjectNamespace(ctx context.Context, orgName string, orgID uuid.UUID) (string, error) {
	projectName := kubename.ProjectName(orgName)
	namespace, err := h.gardener.EnsureProject(ctx, projectName, orgID)
	if err != nil {
		return "", fmt.Errorf("ensure project %s: %w", projectName, err)
	}
	return namespace, nil
}

// clusterToSyncBase builds a ClusterToSync with the common fields shared across
// sync, status checking, and deleted cluster verification.
// Callers set additional fields (ShootName, Deleted, NodePools) as needed.
// cloudProfile/cloudProfileRegion are the catalog regions row (NULL on legacy
// clusters without a region_id - the gardener client falls back to its
// provider defaults then).
func clusterToSyncBase(id uuid.UUID, name, orgName string, orgID uuid.UUID, namespace, region, k8sVersion string, cloudProfile, cloudProfileRegion pgtype.Text) *gardener.ClusterToSync {
	return &gardener.ClusterToSync{
		ID:                 id,
		OrganizationID:     orgID,
		OrganizationName:   orgName,
		Name:               name,
		Namespace:          namespace,
		Region:             region,
		KubernetesVersion:  k8sVersion,
		CloudProfile:       textOrEmpty(cloudProfile),
		CloudProfileRegion: textOrEmpty(cloudProfileRegion),
	}
}

// textOrEmpty unwraps a nullable text column to its string, empty when NULL.
func textOrEmpty(t pgtype.Text) string {
	if t.Valid {
		return t.String
	}
	return ""
}
