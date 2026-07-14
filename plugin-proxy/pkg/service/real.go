package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fundament-oss/fundament/common/gardener"
	pluginsv1 "github.com/fundament-oss/fundament/plugin-controller/pkg/api/v1"
)

// shootAccessProvider is the slice of gardener.AdminKubeconfigCache this
// package consumes; an interface so tests can fake it.
type shootAccessProvider interface {
	AccessFor(ctx context.Context, clusterID string) (*gardener.ShootAccess, error)
}

// GardenerClusterAccess resolves clusters through the shared Gardener
// admin-kubeconfig cache: the controller-runtime client is built from the
// cached shoot access, and the owning organization comes from the shoot's
// fundament.io/organization-id label.
type GardenerClusterAccess struct {
	cache  shootAccessProvider
	scheme *runtime.Scheme
}

// NewGardenerClusterAccess returns a ClusterAccess backed by the shared cache.
func NewGardenerClusterAccess(cache *gardener.AdminKubeconfigCache) (*GardenerClusterAccess, error) {
	scheme := runtime.NewScheme()
	if err := pluginsv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("add plugins scheme: %w", err)
	}
	return &GardenerClusterAccess{cache: cache, scheme: scheme}, nil
}

var _ ClusterAccess = (*GardenerClusterAccess)(nil)

// ForCluster returns a client and owning organization for the cluster. The
// shoot access is cached upstream (AdminKubeconfigCache); the controller-runtime
// client is built per call — this path is the low-frequency MintPluginToken
// lookup, and the client's REST mapper discovers lazily, so a per-call build
// is cheap and avoids a second cache to keep coherent with rotation.
func (g *GardenerClusterAccess) ForCluster(ctx context.Context, clusterID uuid.UUID) (*ClusterTarget, error) {
	access, err := g.cache.AccessFor(ctx, clusterID.String())
	if err != nil {
		return nil, fmt.Errorf("shoot access: %w", err)
	}

	orgID, err := uuid.Parse(access.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("shoot for cluster %s has no valid %s label: %w",
			clusterID, gardener.LabelOrganizationID, err)
	}

	c, err := client.New(access.RESTConfig, client.Options{Scheme: g.scheme})
	if err != nil {
		return nil, fmt.Errorf("create shoot client for cluster %s: %w", clusterID, err)
	}

	return &ClusterTarget{Client: c, OrganizationID: orgID}, nil
}
