package shootverify

import (
	"context"
	"fmt"
	"sync"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/fundament-oss/fundament/common/gardener"
)

// shootAccessSource is the slice of *gardener.AdminKubeconfigCache the
// Gardener verifier uses, as a seam for tests.
type shootAccessSource interface {
	AccessFor(ctx context.Context, clusterID string) (*gardener.ShootAccess, error)
}

// GardenerVerifier reviews tokens against the shoot resolved through the
// shared admin-kubeconfig cache: the same path kube-api-proxy and
// plugin-proxy use to reach shoots. The organization comes from the Shoot's
// label, read by the cache when it resolved the shoot.
type GardenerVerifier struct {
	access    shootAccessSource
	clientset func(*rest.Config) (kubernetes.Interface, error)

	mu      sync.Mutex
	clients map[string]cachedClient // by cluster id
}

// cachedClient is the clientset built for one ShootAccess. The cache hands out
// the same *ShootAccess until the credentials rotate, so a different pointer
// means the clientset must be rebuilt.
type cachedClient struct {
	access *gardener.ShootAccess
	cs     kubernetes.Interface
}

// NewGardenerVerifier returns a Verifier backed by the given cache.
func NewGardenerVerifier(cache *gardener.AdminKubeconfigCache) *GardenerVerifier {
	return newGardenerVerifier(cache, newReviewClientset)
}

func newGardenerVerifier(access shootAccessSource, clientset func(*rest.Config) (kubernetes.Interface, error)) *GardenerVerifier {
	return &GardenerVerifier{access: access, clientset: clientset, clients: map[string]cachedClient{}}
}

// Verify implements Verifier.
func (g *GardenerVerifier) Verify(ctx context.Context, clusterID, token, audience string) (*Subject, error) {
	access, err := g.access.AccessFor(ctx, clusterID)
	if err != nil {
		// The cache wraps "no shoot found" and transport failures alike; both
		// are a deny, and the exchange logs the wrapped cause.
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	cs, err := g.clientFor(clusterID, access)
	if err != nil {
		return nil, fmt.Errorf("%w: build shoot clientset: %w", ErrUnavailable, err)
	}
	subject, err := ReviewWithClientset(ctx, cs, token, audience)
	if err != nil {
		return nil, err
	}
	subject.OrganizationID = access.OrganizationID
	return subject, nil
}

// clientFor returns the clientset for the cluster's current credentials,
// building it only when they change.
func (g *GardenerVerifier) clientFor(clusterID string, access *gardener.ShootAccess) (kubernetes.Interface, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if c, ok := g.clients[clusterID]; ok && c.access == access {
		return c.cs, nil
	}
	cs, err := g.clientset(access.RESTConfig)
	if err != nil {
		return nil, err
	}
	g.clients[clusterID] = cachedClient{access: access, cs: cs}
	return cs, nil
}
