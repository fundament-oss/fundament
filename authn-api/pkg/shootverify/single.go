package shootverify

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// SingleClusterVerifier reviews tokens against exactly one cluster, known
// under one fundament cluster id: the local plugin sandbox, from the
// kubeconfig mounted into the pod. Any other cluster id is ErrUnknownCluster.
// The organization it reports is the seeded one, so the exchange's
// cross-check against the database row holds in local dev exactly as it does
// in production.
type SingleClusterVerifier struct {
	cs             kubernetes.Interface
	clusterID      string
	organizationID string
}

// NewSingleClusterVerifier wraps an existing clientset.
func NewSingleClusterVerifier(cs kubernetes.Interface, clusterID, organizationID string) *SingleClusterVerifier {
	return &SingleClusterVerifier{cs: cs, clusterID: clusterID, organizationID: organizationID}
}

// NewKubeconfigVerifier builds a SingleClusterVerifier from a kubeconfig on
// disk (the plugin sandbox Secret mounted into the pod).
func NewKubeconfigVerifier(kubeconfigPath, clusterID, organizationID string) (*SingleClusterVerifier, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("load sandbox kubeconfig %q: %w", kubeconfigPath, err)
	}
	cs, err := newReviewClientset(cfg)
	if err != nil {
		return nil, fmt.Errorf("sandbox: %w", err)
	}
	return NewSingleClusterVerifier(cs, clusterID, organizationID), nil
}

// Verify implements Verifier.
func (s *SingleClusterVerifier) Verify(ctx context.Context, clusterID, token, audience string) (*Subject, error) {
	if clusterID != s.clusterID {
		return nil, fmt.Errorf("%w: %s is not the verifier's cluster", ErrUnknownCluster, clusterID)
	}
	subject, err := ReviewWithClientset(ctx, s.cs, token, audience)
	if err != nil {
		return nil, err
	}
	subject.OrganizationID = s.organizationID
	return subject, nil
}
